package dockercli

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"time"
)

// ImageRemovalTarget pins the user's selection. Only explicit repository:tag
// removal is supported; Docker may retain layers when another tag references
// the same image. No dangling/untagged deletion or --force option is exposed.
type ImageRemovalTarget struct {
	Context    string
	Repository string
	Tag        string
	ID         string
}

func (t ImageRemovalTarget) Reference() string { return t.Repository + ":" + t.Tag }

type ImageRemovalResult struct {
	Outcome string // "blocked", "not_removed", "verified", or "unverified"
	Reason  string // fixed, safe UI wording only; never raw Docker stderr
}

var imageRemovalRepo = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:/-]{0,239}$`)
var imageRemovalTag = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_.-]{0,127}$`)
var imageRemovalContext = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,127}$`)
var imageRemovalID = regexp.MustCompile(`^[a-fA-F0-9]{12,64}$`)

func normalizedImageID(value string) string {
	return strings.TrimPrefix(value, "sha256:")
}

func ValidateImageRemovalTarget(t ImageRemovalTarget) error {
	if !imageRemovalContext.MatchString(t.Context) ||
		!imageRemovalRepo.MatchString(t.Repository) || t.Repository == "<none>" ||
		!imageRemovalTag.MatchString(t.Tag) || t.Tag == "<none>" ||
		!imageRemovalID.MatchString(normalizedImageID(t.ID)) {
		return errors.New("unsupported image removal target")
	}
	return nil
}

func imageMatchesSelectedID(foundID, selectedID string) bool {
	found := strings.ToLower(normalizedImageID(foundID))
	selected := strings.ToLower(normalizedImageID(selectedID))
	return imageRemovalID.MatchString(found) &&
		len(found) >= len(selected) && strings.HasPrefix(found, selected)
}

func parseImageListing(raw []byte) ([]Image, error) {
	if len(raw) > 4*1024*1024 {
		return nil, errors.New("image listing exceeded limit")
	}
	images, err := decodeLines[Image](raw)
	if err != nil || len(images) > 10000 {
		return nil, errors.New("image listing could not be verified")
	}
	return images, nil
}

func selectedImageCount(images []Image, target ImageRemovalTarget) (count int, sameID bool) {
	for _, image := range images {
		if image.Repository == target.Repository && image.Tag == target.Tag {
			count++
			if imageMatchesSelectedID(image.ID, target.ID) {
				sameID = true
			}
		}
	}
	return count, sameID
}

// imageRemovalErrorReason classifies a known container-reference conflict into
// fixed UI wording. Docker stderr and error strings are never returned to the TUI.
func imageRemovalErrorReason(err error) string {
	message := strings.ToLower(err.Error())
	if strings.Contains(message, "container") &&
		(strings.Contains(message, "is being used by") ||
			strings.Contains(message, "image is in use by") ||
			strings.Contains(message, "is using its referenced image")) {
		return "Docker refused removal because an existing container references this image."
	}
	return "Docker reported an error while attempting image removal."
}

// RemoveImageTag is only reached after UI approval. It checks the active
// context and exact image reference/ID again before dispatch. Docker has no
// atomic compare-and-delete, so changes between the last read and rm remain
// possible; those races cannot be ruled out by this preflight alone.
func (c Client) RemoveImageTag(ctx context.Context, target ImageRemovalTarget) ImageRemovalResult {
	blocked := func(reason string) ImageRemovalResult {
		return ImageRemovalResult{Outcome: "blocked", Reason: reason}
	}
	unverified := func(reason string) ImageRemovalResult {
		return ImageRemovalResult{Outcome: "unverified", Reason: reason}
	}
	if ValidateImageRemovalTarget(target) != nil {
		return blocked("Unsupported image target; no removal attempted.")
	}
	ctx, cancel := context.WithTimeout(ctx, 35*time.Second)
	defer cancel()

	current, err := c.Runner.Run(ctx, "context", "show")
	if err != nil || strings.TrimSpace(string(current)) != target.Context {
		return blocked("Docker context changed or is unavailable; no removal attempted.")
	}
	listArgs := []string{"--context", target.Context, "image", "ls", "--no-trunc", "--format", "{{json .}}"}
	raw, err := c.Runner.Run(ctx, listArgs...)
	if err != nil {
		return blocked("Cannot verify the current image list; no removal attempted.")
	}
	images, err := parseImageListing(raw)
	if err != nil {
		return blocked("Cannot validate the image list; no removal attempted.")
	}
	count, same := selectedImageCount(images, target)
	if count != 1 || !same {
		return blocked("The selected image reference or ID changed; refresh and review again.")
	}
	// The explicit -- ends option parsing. Never pass --force or --no-prune.
	_, removalErr := c.Runner.Run(ctx, "--context", target.Context, "image", "rm", "--", target.Reference())

	// Read back even if Docker reports an error. Never infer final image state
	// from the CLI exit status alone, and never render untrusted CLI stderr.
	raw, err = c.Runner.Run(ctx, listArgs...)
	if err != nil {
		return unverified("Image removal was attempted, but image-list readback failed; refresh before retrying.")
	}
	images, err = parseImageListing(raw)
	if err != nil {
		return unverified("Image removal was attempted, but image-list readback was invalid; refresh before retrying.")
	}
	count, same = selectedImageCount(images, target)
	if count > 1 || (count == 1 && !same) {
		return unverified("The image reference changed or is ambiguous after the attempt; refresh before retrying.")
	}
	if count == 0 {
		if removalErr != nil {
			return ImageRemovalResult{
				Outcome: "verified",
				Reason:  "Docker reported an error, but readback confirms that the selected image tag is absent. This does not prove the command succeeded.",
			}
		}
		return ImageRemovalResult{Outcome: "verified", Reason: "Readback confirms the selected image tag is absent."}
	}
	if removalErr != nil {
		return ImageRemovalResult{
			Outcome: "not_removed",
			Reason:  imageRemovalErrorReason(removalErr) + " Readback confirms the selected tag still points to the same image.",
		}
	}
	return unverified("Docker reported success, but readback still lists the selected image tag; refresh before retrying.")
}
