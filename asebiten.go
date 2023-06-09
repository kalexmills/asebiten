package asebiten

import (
	"errors"
	"github.com/hajimehoshi/ebiten/v2"
	"golang.org/x/exp/maps"
	"golang.org/x/image/draw"
	"image"
	"log"
	"sync"
)

// Animation is a collection of animations, keyed by a name called a 'tag'. Each tagged animation starts from its first
// frame and runs until its last frame before looping back to the beginning. Use Callback to take action at the end of a
// frame. Animation is not thread-safe, but all Callbacks are run synchronously.
//
// Every Animation has an empty tag which loops through every frame in the Sprite Sheet in order. This is the default
// animation which will be played.
type Animation struct {
	paused    bool
	currTag   string
	currFrame int

	callbacks map[string]Callback

	// FramesByTagName lists all frames, keyed by their tag. Take care when editing the images associated with this map,
	// as Asebiten uses subimages for each tag, even when that's redundant.
	FramesByTagName map[string][]AniFrame

	// Source is a struct representing the raw JSON read from the Aesprite SpriteSheet on import. Cast to the correct
	// version's SpriteSheet model to use.
	Source SpriteSheet

	gpuFrame  *ebiten.Image
	needsDraw bool

	elapsedMillis float64
}

func (r Rect) ImageRect() image.Rectangle {
	return image.Rect(r.X, r.Y, r.X+r.W, r.Y+r.H)
}

type Rect struct {
	X int `json:"x"`
	Y int `json:"y"`
	Size
}

type Size struct {
	W int `json:"w"`
	H int `json:"h"`
}

var tpsMut sync.Once
var TPS int

// SetTPS is automatically called before the first animation is drawn to screen. It must be explicitly called again
// anytime that TPS is changed.
func SetTPS() {
	TPS = ebiten.TPS()
}

// Clone creates a shallow clone of this animation which uses the same SpriteSheet as the original, but gets its own
// callbacks and state. The tag, frame, and callbacks set on the source animation are copied for convenience. All timing
// information is reset at the time the Animation is cloned.
func (a *Animation) Clone() Animation {
	return Animation{
		FramesByTagName: a.FramesByTagName,
		callbacks:       maps.Clone(a.callbacks),
		currTag:         a.currTag,
		currFrame:       a.currFrame,
		paused:          a.paused,
	}
}

// NewFlyweightAnimation creates a new animation which uses the SpriteSheet already loaded up in the provided animation.
func NewFlyweightAnimation(source *Animation) Animation {
	return Animation{
		FramesByTagName: source.FramesByTagName,
	}
}

// Callback is used for animation callbacks, which are triggered whenever an animation runs out of frames. All callbacks
// are run synchronously on the same thread where Animation.Update() is called.
type Callback func(*Animation)

// NewAnimation creates a new Animation using the provided map from tag names to a list of frames to run. If a nil map
// is passed in this func also returns nil.
func NewAnimation(anim map[string][]AniFrame) *Animation {
	if anim == nil {
		return nil
	}
	result := &Animation{
		FramesByTagName: anim,
		callbacks:       make(map[string]Callback),
		currTag:         "",
		currFrame:       0,
	}
	var isCPUSprite bool
	var rect image.Rectangle
	for _, frames := range anim {
		for _, frame := range frames {
			if _, ok := frame.Image.(*ebiten.Image); !ok {
				isCPUSprite = true
			}
			rect = rect.Union(frame.SourceRect.Bounds())
		}
	}
	if isCPUSprite {
		result.gpuFrame = ebiten.NewImage(rect.Dx(), rect.Dy())
	}
	return result
}

// Pause pauses a currently running animation. Animations are running by default.
func (a *Animation) Pause() {
	a.paused = true
}

// Resume resumes a previously paused animation. Animations are running by default.
func (a *Animation) Resume() {
	a.paused = false
}

func (a *Animation) SetFrame(idx int) error {
	if idx < 0 || len(a.FramesByTagName[a.currTag]) <= idx {
		return errors.New("frame index out of bounds")
	}
	a.needsDraw = true
	a.currFrame = idx
	return nil
}

// Toggle toggles the running state of this animation; if running it pauses, if paused, it resumes.
func (a *Animation) Toggle() {
	a.paused = !a.paused
}

// Restart restarts the currently running animation from the beginning.
func (a *Animation) Restart() {
	a.currFrame = 0
	a.needsDraw = true
}

// SetTag sets the currently running tag to the provided tag name. If the tag name is different from the currently
// running tag, this func also sets the frame number to 0.
func (a *Animation) SetTag(tag string) {
	if a.currTag != tag {
		a.currFrame = 0
	}
	a.currTag = tag
	a.needsDraw = true
}

// OnEnd registers the provided Callback to run on the same frame that the final frame of the animation  is crossed.
// Each Callback is called only once every time the animation ends, even if the animation ends multiple times during a
// single frame. Callbacks for a given tag can be disabled by calling OnEnd(tag, nil).
//
// Note: for "reverse" or "pingpong" animations, the end of the animation is defined as the end of the sequence of
// frames stored by asebiten.
func (a *Animation) OnEnd(tag string, callback Callback) {
	a.callbacks[tag] = callback
}

// Update should be called once on every running animation each frame, only after calling asebiten.Update(). Calling
// Update() on a paused animation immediately returns.
func (a *Animation) Update() {
	if a.paused {
		return
	}
	tpsMut.Do(func() {
		SetTPS()
	})

	a.elapsedMillis += 1 / float64(TPS) * 1000

	// advance the current frame until you can't; this loop usually runs only once per tick
	for a.elapsedMillis > float64(a.FramesByTagName[a.currTag][a.currFrame].DurationMillis) {
		prior := a.elapsedMillis
		a.elapsedMillis -= float64(a.FramesByTagName[a.currTag][a.currFrame].DurationMillis)
		log.Println("elapsedMillis: ", prior, " - ", a.FramesByTagName[a.currTag][a.currFrame].DurationMillis, " = ", a.elapsedMillis)
		a.currFrame = (a.currFrame + 1) % len(a.FramesByTagName[a.currTag])
		if a.gpuFrame != nil {
			a.needsDraw = true
		}
		if a.currFrame != 0 || a.callbacks[a.currTag] == nil {
			continue
		}
		a.callbacks[a.currTag](a)
	}
	return
}

// DrawTo draws an animation from to the provided screen using the provided options. Does not automatically perform
// translation for packed sprite sheets, since doing so requires modifying GeoM prior to other transformations. See
// DrawPackedTo for that functionality.
func (a *Animation) DrawTo(screen *ebiten.Image, options *ebiten.DrawImageOptions) {
	frame := a.FramesByTagName[a.currTag][a.currFrame]
	if a.gpuFrame == nil {
		screen.DrawImage(frame.Image.(*ebiten.Image), options)
		return
	}
	if a.needsDraw {
		a.gpuFrame.Clear()
		draw.Draw(a.gpuFrame, frame.SourceRect, frame.Image, frame.Image.Bounds().Min, draw.Over)
		a.needsDraw = false
	}
	screen.DrawImage(a.gpuFrame, options)
}

// DrawPackedTo draws a packed animation to the proveded screen. A func to manage any draw options is provided -- the
// translations needed to unpack frames from packed sprite sheets have already been performed.
func (a *Animation) DrawPackedTo(screen *ebiten.Image, optFunc func(options *ebiten.DrawImageOptions)) {
	opts := ebiten.DrawImageOptions{}
	frame := a.FramesByTagName[a.currTag][a.currFrame]
	opts.GeoM.Translate(float64(frame.SourceRect.Min.X), float64(frame.SourceRect.Min.Y))
	optFunc(&opts)
	if a.gpuFrame == nil {
		screen.DrawImage(frame.Image.(*ebiten.Image), &opts)
		return
	}
	if a.needsDraw {
		a.gpuFrame.Clear()
		draw.Draw(a.gpuFrame, a.gpuFrame.Bounds(), frame.Image, frame.Image.Bounds().Min, draw.Over)
		a.needsDraw = false
	}
	screen.DrawImage(a.gpuFrame, &opts)
}

// Bounds retrieves the bounds of the current frame.
func (a *Animation) Bounds() image.Rectangle {
	return a.FramesByTagName[a.currTag][a.currFrame].Image.Bounds()
}

// FrameIdx retrieves the index of the current frame.
func (a *Animation) FrameIdx() int {
	return a.currFrame
}

// Frame retrieves the current frame for the provided animation.
func (a *Animation) Frame() AniFrame {
	return a.FramesByTagName[a.currTag][a.currFrame]
}

// AniFrame denotes a single frame of this animation.
type AniFrame struct {
	// FrameIdx is the original index of this frame from Aseprite.
	FrameIdx int
	// Image represents an image to use. For efficiency, it's recommended to use subimage for each frame.
	Image image.Image
	// DurationMillis represents the number of milliseconds this frame should be shown.
	DurationMillis int64
	// SourceRect is the source rectangle in the sprite sheet. Primarily used for packed sprites.
	SourceRect image.Rectangle
}
