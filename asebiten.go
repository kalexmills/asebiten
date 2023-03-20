package asebiten

import (
	"github.com/hajimehoshi/ebiten/v2"
	"golang.org/x/exp/maps"
	"time"
)

var (
	lastTick = time.Now()
	// TotalMillis the number of milliseconds elapsed since the game started. It is exported for flexibility. Avoid
	// modifying it directly unless you know what you're doing.
	TotalMillis int64
	// DeltaMillis is the number of milliseconds elapsed since the last frame, in games which call Update() at the start
	// of each frame. It is exported for flexibility. Avoid modifying it directly unless you know what you're doing.
	DeltaMillis int64
)

// Animation is a collection of animations, keyed by a name called a 'tag'. Each tagged animation starts from its first
// frame and runs until its last frame before looping back to the beginning. Use Callback to take action at the end of a
// frame. Animation is not thread-safe, but all Callbacks are run synchronously.
//
// Every Animation has an empty tag which loops through every frame in the Sprite Sheet in order. This is the default
// animation which will be played.
type Animation struct {
	paused          bool
	framesByTagName map[string][]Frame
	currTag         string
	currFrame       int

	accumMillis int64
	callbacks   map[string]Callback

	// Source is a struct representing the raw JSON read from the Aesprite SpriteSheet on import. Cast to the correct
	// version's SpriteSheet model to use.
	Source any
}

// Clone creates a shallow clone of this animation which uses the same SpriteSheet as the original, but gets its own
// callbacks and state. The tag, frame, and callbacks set on the source animation are copied for convenience. All timing
// information is reset at the time the Animation is cloned.
func (a *Animation) Clone() *Animation {
	return &Animation{
		framesByTagName: a.framesByTagName,
		callbacks:       maps.Clone(a.callbacks),
		currTag:         a.currTag,
		currFrame:       a.currFrame,
		paused:          a.paused,
	}
}

// Callback is used for animation callbacks, which are triggered whenever an animation runs out of frames. All callbacks
// are run synchronously on the same thread where Animation.Update() is called.
type Callback func(*Animation)

// NewAnimation creates a new Animation using the provided map from tag names to a list of frames to run. If a nil map
// is passed in this func also returns nil.
func NewAnimation(anim map[string][]Frame) *Animation {
	if anim == nil {
		return nil
	}
	result := &Animation{
		framesByTagName: anim,
		callbacks:       make(map[string]Callback),
		currTag:         "",
		currFrame:       0,
	}
	return result
}

// Update should be called once at the beginning of every frame to updated DeltaMillis and TotalMillis. It measures
// time elapsed since the last frame.
func Update() {
	now := time.Now()
	DeltaMillis = now.Sub(lastTick).Milliseconds()
	TotalMillis += DeltaMillis
	lastTick = now
}

// Pause pauses a currently running animation. Animations are running by default.
func (a *Animation) Pause() {
	a.paused = true
}

// Resume resumes a previously paused animation. Animations are running by default.
func (a *Animation) Resume() {
	a.paused = false
}

// Toggle toggles the running state of this animation; if running it pauses, if paused, it resumes.
func (a *Animation) Toggle() {
	a.paused = !a.paused
}

// Restart restarts the currently running animation from the beginning.
func (a *Animation) Restart() {
	a.currFrame = 0
}

// SetTag sets the currently running tag to the provided tag name. If the tag name is different from the currently
// running tag, this func also sets the frame number to 0.
func (a *Animation) SetTag(tag string) {
	if a.currTag != tag {
		a.currFrame = 0
	}
	a.currTag = tag
}

// OnEnd registers the provided Callback to run on the same frame that the animation end frame is crossed. Each Callback
// is called only once everytime the animation ends, even if the animation ends multiple times in a single frame.
// Callbacks for a given tag can be disabled by calling OnEnd(tag, nil).
func (a *Animation) OnEnd(tag string, callback Callback) {
	a.callbacks[tag] = callback
}

// Update should be called once on every running animation each frame, only after calling asebiten.Update(). Calling
// Update() on a paused animation immediately returns.
func (a *Animation) Update() {
	if a.paused {
		return
	}
	a.accumMillis += DeltaMillis

	// advance the current frame until you can't; this loop usually runs only once per tick
	for a.accumMillis > a.framesByTagName[a.currTag][a.currFrame].DurationMillis {
		a.accumMillis -= a.framesByTagName[a.currTag][a.currFrame].DurationMillis
		a.currFrame = (a.currFrame + 1) % len(a.framesByTagName[a.currTag])
		if a.currFrame != 0 || a.callbacks[a.currTag] == nil {
			continue
		}
		a.callbacks[a.currTag](a)
	}
	return
}

// DrawTo draws this animation to the provided screen using the provided options.
func (a *Animation) DrawTo(screen *ebiten.Image, options *ebiten.DrawImageOptions) {

	frame := a.framesByTagName[a.currTag][a.currFrame]
	screen.DrawImage(frame.Image, options)
}

// Frame denotes a single frame of this animation.
type Frame struct {
	// Image represents an image to use. For efficiency, it's recommended to use subimage for each frame.
	Image *ebiten.Image
	// DurationMillis represents the number of milliseconds this frame should be shown.
	DurationMillis int64
}