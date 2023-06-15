package asebiten

import (
	"encoding/json"
	"fmt"
	"github.com/hajimehoshi/ebiten/v2"
	"image"
	"io/fs"
	"path"
	"strings"
)

// SpriteSheet represents the json export format for an Aesprite sprite sheet, which has been exported with frames in an
// *Array*.
type SpriteSheet struct {
	Frames []*Frame `json:"frames"`
	Meta   Meta     `json:"meta"`

	// Image is the image referred to by the SpriteSheet.
	Image *ebiten.Image

	// Animations stores anmiations ready to go; keyed by frameTag. If no frametags are used the
	// entire sprite sheet is available under a single animation keyed by the empty string.
	Animations map[string]Animation
}

type Meta struct {
	App       string     `json:"app"`
	Version   string     `json:"version"`
	Image     string     `json:"image"`
	Format    string     `json:"format"`
	Size      Size       `json:"size"`
	Scale     string     `json:"scale"`
	FrameTags []FrameTag `json:"frameTags"`
	Layers    []Layer    `json:"layers"`
	Slices    []Slice    `json:"slices"`
}

type Slice struct {
	Name string     `json:"name"`
	Keys []SliceKey `json:"keys"`
}

type SliceKey struct {
	Frame  int  `json:"frame"`
	Bounds Rect `json:"bounds"`
}

type FrameTag struct {
	Name      string `json:"name"`
	From      int    `json:"from"`
	To        int    `json:"to"`
	Direction string `json:"direction"`
	Color     string `json:"color"`
}

type Layer struct {
	Name      string `json:"name"`
	Opacity   byte   `json:"opacity"`
	BlendMode string `json:"blendMode"`
}

type Frame struct {
	Frame            Rect `json:"frame"`
	Rotated          bool `json:"rotated"`
	Trimmed          bool `json:"trimmed"`
	SpriteSourceSize Rect `json:"spriteSourceSize"`
	SourceSize       Size `json:"sourceSize"`
	Duration         int  `json:"duration"`
}

// LoadAnimation loads a sprite from the provided filesystem, based on the provided json path. The image paths are
// assumed to be found in the directory relative to the path passed in.
func LoadAnimation(fs fs.FS, jsonPath string) (*Animation, error) {
	sheet, err := LoadSpriteSheet(fs, jsonPath)
	if err != nil {
		return nil, err
	}
	var byTagName map[string][]AniFrame
	if len(sheet.Meta.FrameTags) == 0 {
		byTagName, err = loadNoTags(&sheet)
	} else {
		byTagName, err = loadWithTags(&sheet)
	}
	if err != nil {
		return nil, err
	}
	result := NewAnimation(byTagName)
	result.Source = sheet
	return result, nil
}

func loadNoTags(sheet *SpriteSheet) (map[string][]AniFrame, error) {
	byTagName := make(map[string][]AniFrame)
	for idx, frame := range sheet.Frames {
		img := ebiten.NewImageFromImage(sheet.Image.SubImage(frame.Frame.ImageRect()))
		byTagName[""] = append(byTagName[""], AniFrame{
			FrameIdx:       idx,
			Image:          img,
			DurationMillis: int64(frame.Duration),
		})
	}
	return byTagName, nil
}

func loadWithTags(sheet *SpriteSheet) (map[string][]AniFrame, error) {
	byTagName, err := loadNoTags(sheet)
	if err != nil {
		return nil, err
	}
	imgCache := make(map[int]*ebiten.Image)
	for _, tag := range sheet.Meta.FrameTags {
		for i := tag.From; i <= tag.To; i++ {
			frame := sheet.Frames[i]
			img, ok := imgCache[i]
			if !ok {
				img = ebiten.NewImageFromImage(sheet.Image.SubImage(frame.Frame.ImageRect()))
				imgCache[i] = img
			}
			byTagName[tag.Name] = append(byTagName[tag.Name], AniFrame{
				FrameIdx:       i,
				Image:          img,
				DurationMillis: int64(frame.Duration),
			})
		}
		switch tag.Direction {
		case "reverse":
			byTagName[tag.Name] = reverse(byTagName[tag.Name])
		case "pingpong":
			byTagName[tag.Name] = pingpong(byTagName[tag.Name])
		case "pingpong_reverse":
			byTagName[tag.Name] = reverse(pingpong(byTagName[tag.Name]))
		}
	}
	return byTagName, nil
}

// LoadSpriteSheet only loads sprite sheet metadata for use in whatever manner the caller would prefer.
// If you want an asebiten.Animation, you should probably use LoadAnimation instead.
func LoadSpriteSheet(fs fs.FS, jsonPath string) (SpriteSheet, error) {
	sheet, err := fs.Open(jsonPath)
	if err != nil {
		return SpriteSheet{}, err
	}
	defer sheet.Close()
	var result SpriteSheet
	if err := json.NewDecoder(sheet).Decode(&result); err != nil {
		return SpriteSheet{}, err
	}
	if !strings.HasPrefix(result.Meta.Version, "1.3") {
		return SpriteSheet{}, fmt.Errorf("version mismatch; expected 1.3, got %s", result.Meta.Version)
	}
	result.Image, err = loadImage(fs, jsonPath, &result)
	return result, err
}

func loadImage(fs fs.FS, jsonPath string, sheet *SpriteSheet) (*ebiten.Image, error) {
	reader, err := fs.Open(path.Join(path.Dir(jsonPath), sheet.Meta.Image))
	if err != nil {
		return nil, err
	}
	img, _, err := image.Decode(reader)
	if err != nil {
		return nil, err
	}
	return ebiten.NewImageFromImage(img), nil
}

func reverse(frames []AniFrame) []AniFrame {
	n := len(frames) - 1
	for i := 0; i < len(frames)/2; i++ {
		frames[i], frames[n-i] = frames[n-i], frames[i]
	}
	return frames
}

func pingpong(frames []AniFrame) []AniFrame {
	for i := len(frames) - 2; i >= 1; i-- {
		frames = append(frames, frames[i])
	}
	return frames
}
