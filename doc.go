// Package spatial computes linear speaker gains for channel-based bed content
// and dynamic objects in a listener-local 3D coordinate system.
//
// It supports a standard 5.1.4 speaker layout and a simplified two-channel
// binaural fold. The package produces a deterministic render plan; callers
// apply the returned gains to audio samples.
package spatial
