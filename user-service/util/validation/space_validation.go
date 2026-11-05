package validation

import (
	"errors"
	"fmt"
	"math"
	"regexp"
	"strings"
	"unicode/utf8"

	"social-network-go/user-service/model"
)

var (
	hexColorRegex = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

	validSpaceThemes = map[string]bool{
		"COSMIC_VOID":      true,
		"ART_GALLERY":      true,
		"BLUEPRINT_STUDIO": true,
		"ETHEREAL_GLASS":   true,
	}

	validViewModes = map[string]bool{
		"SPACE": true,
		"FEED":  true,
	}

	validNodeTypes = map[string]bool{
		"IDENTITY_CORE":  true,
		"POST_CARD":      true,
		"COMMENT_CARD":   true,
		"MEDIA_FRAME":    true,
		"TEXT_NOTE":      true,
		"AUDIO_VINYL":    true,
		"PORTAL_LINK":    true,
		"GUESTBOOK_WALL": true,
		"EMBED_IFRAME":   true,
	}

	validFrameStyles = map[string]bool{
		"GLASS":           true,
		"GALLERY_PLAQUE":  true,
		"COSMIC_ORB":      true,
		"MINIMAL":         true,
		"POLAROID":        true,
		"MUSEUM_FRAME":    true,
		"FLOATING_GLASS":  true,
		"MANUSCRIPT":      true,
		"QUOTE_CARD":      true,
		"GLASS_BUBBLE":    true,
		"MUSEUM_PLAQUE":   true,
		"GALLERY_MATTE":   true,
		"BORDERLESS":      true,
		"TAPE_PIN":        true,
		"SERIF_EDITORIAL": true,
		"MONO_TECH":       true,
		"HANDWRITTEN":     true,
		"CLASSIC_VINYL":   true,
		"NEON_DISC":       true,
		"STAR_PORTAL":     true,
		"CORK_BOARD":      true,
		"BROWSER_WINDOW":  true,
		"CINEMA_SCREEN":   true,
		"GLASS_PORTAL":    true,
	}

	validGuestbookCardColors = map[string]bool{
		"AMBER":   true,
		"ROSE":    true,
		"CYAN":    true,
		"VIOLET":  true,
		"EMERALD": true,
	}
)

// ValidateSpaceConfig strictly validates all fields of UpdateSpaceConfigRequest without silent fallbacks
func ValidateSpaceConfig(req model.UpdateSpaceConfigRequest) error {
	if !validSpaceThemes[req.Theme] {
		return fmt.Errorf("invalid theme: %s", req.Theme)
	}
	if !validViewModes[req.DefaultViewMode] {
		return fmt.Errorf("invalid defaultViewMode: %s", req.DefaultViewMode)
	}
	if !hexColorRegex.MatchString(req.CustomAccent) {
		return fmt.Errorf("invalid customAccent hex color: %s", req.CustomAccent)
	}
	if req.AllowGuestbook == nil {
		return errors.New("allowGuestbook is required")
	}
	if req.Nodes == nil {
		return errors.New("nodes array is required")
	}
	if len(req.Nodes) > model.MaxSpatialNodes {
		return fmt.Errorf("nodes count exceeds maximum limit of %d", model.MaxSpatialNodes)
	}

	seenIDs := make(map[string]bool, len(req.Nodes))
	hasIdentityCore := false

	for i, n := range req.Nodes {
		if strings.TrimSpace(n.ID) == "" || len(n.ID) > 64 {
			return fmt.Errorf("node[%d]: invalid id", i)
		}
		if seenIDs[n.ID] {
			return fmt.Errorf("node[%d]: duplicate node id '%s'", i, n.ID)
		}
		seenIDs[n.ID] = true

		if !validNodeTypes[n.Type] {
			return fmt.Errorf("node[%d]: invalid type '%s'", i, n.Type)
		}
		if n.Type == "IDENTITY_CORE" {
			hasIdentityCore = true
		}

		if math.IsNaN(n.X) || math.IsInf(n.X, 0) || n.X < model.MinCanvasCoord || n.X > model.MaxCanvasCoord {
			return fmt.Errorf("node[%d]: coordinate x out of bounds [%.0f, %.0f]", i, model.MinCanvasCoord, model.MaxCanvasCoord)
		}
		if math.IsNaN(n.Y) || math.IsInf(n.Y, 0) || n.Y < model.MinCanvasCoord || n.Y > model.MaxCanvasCoord {
			return fmt.Errorf("node[%d]: coordinate y out of bounds [%.0f, %.0f]", i, model.MinCanvasCoord, model.MaxCanvasCoord)
		}
		if math.IsNaN(n.Scale) || math.IsInf(n.Scale, 0) || n.Scale < model.MinNodeScale || n.Scale > model.MaxNodeScale {
			return fmt.Errorf("node[%d]: scale out of bounds [%.1f, %.1f]", i, model.MinNodeScale, model.MaxNodeScale)
		}
		if math.IsNaN(n.Rotation) || math.IsInf(n.Rotation, 0) || n.Rotation < model.MinNodeRotation || n.Rotation > model.MaxNodeRotation {
			return fmt.Errorf("node[%d]: rotation out of bounds [%.0f, %.0f]", i, model.MinNodeRotation, model.MaxNodeRotation)
		}
		if n.ZIndex < 0 || n.ZIndex > 1000 {
			return fmt.Errorf("node[%d]: zIndex out of bounds [0, 1000]", i)
		}
		if !validFrameStyles[n.FrameStyle] {
			return fmt.Errorf("node[%d]: invalid frameStyle '%s'", i, n.FrameStyle)
		}
		if n.Content == nil {
			return fmt.Errorf("node[%d]: content map is required", i)
		}
	}

	if !hasIdentityCore {
		return errors.New("spatial canvas must contain at least one IDENTITY_CORE node")
	}

	trimmedAutoplay := strings.TrimSpace(req.AutoplayNodeID)
	if trimmedAutoplay != "" {
		if len(trimmedAutoplay) > 64 {
			return errors.New("autoplayNodeId is too long")
		}
		if !seenIDs[trimmedAutoplay] {
			return fmt.Errorf("autoplayNodeId '%s' does not exist in nodes list", trimmedAutoplay)
		}
	}

	return nil
}

// ValidateGuestbookEntry strictly validates a visitor guestbook note request
func ValidateGuestbookEntry(req model.CreateSpaceGuestbookRequest) error {
	trimmed := strings.TrimSpace(req.Message)
	runeCount := utf8.RuneCountInString(trimmed)
	if runeCount == 0 || runeCount > model.MaxGuestbookMessageLen {
		return fmt.Errorf("guestbook message must be between 1 and %d characters", model.MaxGuestbookMessageLen)
	}
	if !validGuestbookCardColors[req.CardColor] {
		return fmt.Errorf("invalid cardColor: %s", req.CardColor)
	}
	if math.IsNaN(req.PosX) || math.IsInf(req.PosX, 0) || req.PosX < model.MinCanvasCoord || req.PosX > model.MaxCanvasCoord {
		return errors.New("posX out of valid canvas bounds")
	}
	if math.IsNaN(req.PosY) || math.IsInf(req.PosY, 0) || req.PosY < model.MinCanvasCoord || req.PosY > model.MaxCanvasCoord {
		return errors.New("posY out of valid canvas bounds")
	}
	return nil
}
