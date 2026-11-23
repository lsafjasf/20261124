package timemap_test

import (
	"fmt"

	"timemap/timemap"
)

// ExampleBuild maps a 24fps source onto a 30fps output using a speed step:
// 1x for the first second, then 2x for the second.
func ExampleBuild() {
	spec := timemap.Spec{
		SourceRate:   timemap.Rate24,
		SourceFrames: 72,
		AudioRate:    timemap.Rate48000,
		OutputRate:   timemap.Rate30,
		OutputFrames: 60,
		Keyframes: []timemap.Keyframe{
			timemap.KF(0, 1, 0, 1),
			timemap.KF(1, 1, 1, 1),
			timemap.KF(2, 1, 3, 1),
		},
	}
	table, err := timemap.Build(spec)
	if err != nil {
		panic(err)
	}

	for _, n := range []int64{0, 30, 45, 59} {
		frame, _ := table.SourceFrame(n)
		sample, _ := table.AudioSample(n)
		fmt.Printf("out=%d srcFrame=%d audioSample=%d\n", n, frame, sample)
	}
	// Output:
	// out=0 srcFrame=0 audioSample=0
	// out=30 srcFrame=24 audioSample=48000
	// out=45 srcFrame=48 audioSample=96000
	// out=59 srcFrame=70 audioSample=140800
}

// ExampleBuild_rejected shows how an illegal sub-frame loop is reported.
func ExampleBuild_rejected() {
	spec := timemap.Spec{
		SourceRate:   timemap.Rate24,
		SourceFrames: 96,
		AudioRate:    timemap.Rate48000,
		OutputRate:   timemap.Rate30,
		OutputFrames: 90,
		Keyframes: []timemap.Keyframe{
			timemap.KF(0, 1, 0, 1),
			timemap.KF(1, 1, 1, 1),
			timemap.KF(2, 1, 47, 48), // half a source frame backward
			timemap.KF(3, 1, 2, 1),
		},
	}
	_, err := timemap.Build(spec)
	fmt.Println(err)
	// Output:
	// timemap: sub-frame loop rejected
}
