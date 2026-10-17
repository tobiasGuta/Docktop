package dockercli

import (
 "context"
 "errors"
 "fmt"
)

// MCPClientConnectResult separates failure before dispatch from a write
// whose result cannot be independently confirmed. Raw CLI errors or config
// contents are never returned to the terminal UI.
type MCPClientConnectResult struct{
 ClientID, ProfileID, Outcome, Reason string // verified, blocked, unverified
 Updated []MCPClientIntegration
}

// MCPConnectGlobalClient modifies a client's global MCP configuration only
// after verification of the exact client, installed/supported status,
// absence of an existing MCP connection, and existing profile identity.
// This does not run a gateway or authenticate an MCP client. Docker MCP may
// rewrite the client's on-disk config file. External races are not atomic.
func (c Client) MCPConnectGlobalClient(ctx context.Context,clientID,profileID string)MCPClientConnectResult{
 result:=MCPClientConnectResult{ClientID:clientID,ProfileID:profileID,Outcome:"blocked"}
 reject:=func(msg string)MCPClientConnectResult{result.Reason=msg;return result}
 if !identifier(clientID)||!identifier(profileID){
  return reject("Invalid client or profile identifier; no change attempted.")
 }
 profiles,err:=c.MCPProfilesSafe(ctx)
 if err!=nil{return reject("Unable to validate MCP profiles; no change attempted.")}
 found:=false
 for _,p:=range profiles{if p.ID==profileID{found=true;break}}
 if !found{return reject("Selected profile does not exist; no change attempted.")}
 current,err:=c.MCPConfiguredClients(ctx)
 if err!=nil{return reject("Unable to verify client configuration; no change attempted.")}
 var chosen MCPClientIntegration
 known:=false
 for _,entry:=range current{
  if entry.ID==clientID{chosen=entry;known=true;break}
 }
 if !known{return reject("Selected client not in Docker MCP configuration list.")}
 if chosen.Installed!="installed"||chosen.OSSupport!="supported"||chosen.HasConfigError{
  return reject("Docker cannot confirm this client is installed, supported and readable.")
 }
 if chosen.State!="not configured"{
  return reject("Client is already configured or has unknown state. No replacement performed.")
 }
 // Use Cobra's documented global/profile flags. --quiet avoids printing
 // client config or server details; no arguments pass through a shell.
 result.Outcome="unverified" // dispatch boundary: any error could be partial
 _,err=c.run(ctx,"mcp","client","connect","--global","--profile",profileID,"--quiet",clientID)
 if err!=nil{
  result.Reason="Docker MCP connect returned an error. Inspect the client before retrying."
  return result
 }
 after,err:=c.MCPConfiguredClients(ctx)
 if err!=nil{
  result.Reason="Connect command returned success but client readback failed."
  return result
 }
 result.Updated=after
 for _,entry:=range after{
  if entry.ID!=clientID {continue}
  if entry.State=="configured" && entry.Profile==profileID && !entry.HasConfigError{
   result.Outcome="verified"
   result.Reason="Docker MCP client configuration now references the approved profile."
   return result
  }
  result.Reason=fmt.Sprintf("Connect returned success, but requested client/profile association was not verified.")
  return result
 }
 result.Reason="Connect returned success, but client was missing from readback."
 return result
}

// MCPPlausibleGlobalClient is used only to suppress invalid UI actions.
// The backend repeats all checks on the actual write path.
func MCPPlausibleGlobalClient(client MCPClientIntegration)error{
 if !identifier(client.ID){return errors.New("invalid client ID")}
 if client.State!="not configured"{return errors.New("client is configured or state is unknown")}
 if client.Installed!="installed"{return errors.New("client not confirmed installed")}
 if client.OSSupport!="supported"{return errors.New("client OS support not confirmed")}
 if client.HasConfigError{return errors.New("client configuration has an error")}
 return nil
}
