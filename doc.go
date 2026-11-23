// Package tagfix synchronizes the color tags stored in an MP4 container with
// the VUI fields inside its H.264 SPS NAL units.
//
// A patch is prepared completely in memory before the target file is touched.
// The rewritten file is validated by reading both tag locations back, and the
// media payload is compared byte-for-byte with the input. The final write uses
// a temporary file and an atomic rename. If a post-write check fails, a hard
// link backup is atomically restored.
package tagfix
