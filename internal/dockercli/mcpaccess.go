package dockercli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
)

// MCPProfile describes only public-facing identifiers; profile configurations,
// OAuth and secret values are never fetched or rendered by this feature.
type MCPProfile struct{ ID, Name string }
type MCPProfileServer struct{ Name string }

func identifier(raw string) bool {
	return mcpServerID.MatchString(raw) && !strings.HasPrefix(raw, "-")
}

func fieldString(obj map[string]json.RawMessage, keys ...string) string {
	for _, key := range keys {
		var s string
		if json.Unmarshal(obj[key], &s) == nil && s != "" {
			return s
		}
	}
	return ""
}
func jsonObjects(raw []byte, key string) ([]json.RawMessage, error) {
	if len(raw) > 1024*1024 { return nil, errors.New("MCP response too large") }
	var entries []json.RawMessage
	if json.Unmarshal(raw, &entries) == nil { return entries, nil }
	var object map[string]json.RawMessage
	if json.Unmarshal(raw, &object) != nil { return nil, errors.New("unexpected MCP JSON structure") }
	if values, ok := object[key]; ok && json.Unmarshal(values, &entries) == nil {
		return entries, nil
	}
	return nil, errors.New("unsupported MCP JSON response structure; no changes permitted")
}
func parseProfiles(raw []byte) ([]MCPProfile, error) {
	items, err := jsonObjects(raw, "profiles")
	if err != nil { return nil, err }
	seen := map[string]bool{}
	out := make([]MCPProfile, 0, len(items))
	for _, item := range items {
		var obj map[string]json.RawMessage
		if json.Unmarshal(item, &obj) != nil { return nil, errors.New("invalid MCP profile record") }
		id := fieldString(obj, "id", "ID", "profileId", "profile_id")
		if !identifier(id) { return nil, errors.New("unrecognized MCP profile ID: no changes permitted") }
		if seen[id] { return nil, errors.New("duplicate MCP profile ID") }
		seen[id] = true
		name := fieldString(obj, "name", "Name")
		if name == "" { name = id }
		out = append(out, MCPProfile{ID: id, Name: Clean(name, 128)})
		if len(out) > 200 { return nil, errors.New("too many MCP profiles") }
	}
	sort.Slice(out, func(i,j int) bool { return out[i].ID < out[j].ID })
	return out,nil
}
// Docker MCP 0.44.x profile server ls --format json returns SearchResult
// groups: [{id:"profile", name:"profile title", servers:[{snapshot:
// {server:{name:"actual-server"}}}]}]. Never treat the group's "name"
// as a server name; it is the PROFILE display name.
func parseProfileServers(raw []byte, expectedProfile string) ([]MCPProfileServer, error) {
	if !identifier(expectedProfile) { return nil, errors.New("invalid requested MCP profile") }
	items, err := jsonObjects(raw, "servers")
	if err != nil {
		// Some older builds return an envelope containing profile groups.
		items, err = jsonObjects(raw, "profiles")
		if err != nil { return nil, err }
	}
	names := map[string]bool{}
	result := []MCPProfileServer{}
	groupSeen := false
	addMember := func(item json.RawMessage, grouped bool) error {
		var obj map[string]json.RawMessage
		if err := json.Unmarshal(item, &obj); err != nil || obj == nil {
			return errors.New("invalid MCP server entry")
		}
		if _, nestedGroup := obj["servers"]; nestedGroup {
			return errors.New("nested MCP profile groups are unsupported")
		}
		pid := fieldString(obj, "profile", "profileId", "profile_id")
		if pid != "" && pid != expectedProfile {
			return errors.New("MCP server has mismatched profile identity")
		}
		// The nested snapshot is the authoritative server identity in Docker's
		// structured SearchResult. Top-level 'name' is only a compatibility
		// fallback for genuinely flat server records.
		snapshotName := ""
		if snap, ok := obj["snapshot"]; ok {
			var parsed struct {
				Server struct { Name string `json:"name"` } `json:"server"`
			}
			if json.Unmarshal(snap, &parsed) != nil {
				return errors.New("malformed MCP server snapshot")
			}
			snapshotName = parsed.Server.Name
			if !identifier(snapshotName) {
				return errors.New("missing or invalid MCP snapshot server name")
			}
		}
		named := fieldString(obj, "serverName", "server_name", "name")
		if named == "" {
			var nested map[string]json.RawMessage
			if json.Unmarshal(obj["server"], &nested) == nil {
				named = fieldString(nested, "name", "id")
			}
		}
		if snapshotName != "" && named != "" && snapshotName != named {
			return errors.New("MCP server name disagrees with snapshot")
		}
		name := snapshotName
		if name == "" { name = named }
		if grouped && snapshotName == "" {
			return errors.New("profile-grouped MCP server has no verified snapshot name")
		}
		if !identifier(name) { return errors.New("unrecognized MCP server name; no changes permitted") }
		if names[name] { return errors.New("duplicate MCP server identity") }
		names[name] = true
		result = append(result, MCPProfileServer{Name: name})
		if len(result) > 1000 { return errors.New("too many MCP servers") }
		return nil
	}
	for _, item := range items {
		var obj map[string]json.RawMessage
		if err := json.Unmarshal(item, &obj); err != nil || obj == nil {
			return nil, errors.New("invalid MCP profile-server entry")
		}
		if children, isGroup := obj["servers"]; isGroup {
			groupSeen = true
			pid := fieldString(obj, "id", "profile", "profileId", "profile_id")
			if !identifier(pid) { return nil, errors.New("invalid MCP profile group ID") }
			if pid != expectedProfile { continue }
			var members []json.RawMessage
			if err := json.Unmarshal(children, &members); err != nil || members == nil {
				return nil, errors.New("invalid MCP group server array")
			}
			for _, member := range members {
				if err := addMember(member, true); err != nil { return nil, err }
			}
			continue
		}
		// Compatibility with older flat records: [{profile,name}, ...].
		pid := fieldString(obj, "profile", "profileId", "profile_id")
		if pid != "" && pid != expectedProfile { continue }
		if err := addMember(item, false); err != nil { return nil, err }
	}
	if groupSeen && len(items) > 0 && len(result) == 0 {
		// Empty matching group is valid; an unrelated group alone is not.
		found := false
		for _, item := range items {
			var obj map[string]json.RawMessage
			_ = json.Unmarshal(item, &obj)
			if fieldString(obj, "id", "profile", "profileId", "profile_id") == expectedProfile { found = true; break }
		}
		if !found { return nil, errors.New("expected MCP profile group missing") }
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result, nil
}

// MCPProfilesSafe and MCPProfileMembers read only structured metadata.
// They intentionally never call 'profile show' or 'profile config --get-all'.
func (c Client) MCPProfilesSafe(ctx context.Context) ([]MCPProfile,error) {
	raw,err:=c.run(ctx,"mcp","profile","list","--format","json")
	if err!=nil {return nil,err}
	return parseProfiles(raw)
}
func (c Client) MCPProfileMembers(ctx context.Context,profile string) ([]MCPProfileServer,error) {
	if !identifier(profile) {return nil,errors.New("invalid MCP profile ID")}
	raw,err:=c.run(ctx,"mcp","profile","server","ls","--filter","profile="+profile,"--format","json")
	if err!=nil {return nil,err}
	return parseProfileServers(raw,profile)
}

// MCPSetToolAccess changes the *tool allowlist*, not profile membership.
// Exact profile/server membership is re-checked immediately before mutation.
// No gateway, container, client, OAuth, or secret operation is performed.
func (c Client) MCPSetToolAccess(ctx context.Context,profile,server string,enable bool) error {
	if !identifier(profile)||!identifier(server) {return errors.New("invalid profile or server identifier")}
	profiles,err:=c.MCPProfilesSafe(ctx)
	if err!=nil {return fmt.Errorf("cannot verify profile: %w",err)}
	foundProfile:=false
	for _,p:=range profiles {if p.ID==profile {foundProfile=true;break}}
	if !foundProfile {return errors.New("profile no longer exists")}
	members,err:=c.MCPProfileMembers(ctx,profile)
	if err!=nil {return fmt.Errorf("cannot verify server membership: %w",err)}
	foundServer:=false
	for _,m:=range members {if m.Name==server {foundServer=true;break}}
	if !foundServer {return errors.New("server no longer belongs to selected profile")}
	flag:="--disable-all"
	if enable {flag="--enable-all"}
	_,err=c.run(ctx,"mcp","profile","tools",profile,flag,server)
	return err
}
