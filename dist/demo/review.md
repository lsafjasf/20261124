# VobSub review

Source: `dist/demo/source/demo.idx` + `dist/demo/source/demo.sub`  
Frame: `720x480`  
Method: container metadata only. Text was **not** OCRed or guessed. Every cue below is attached to its decoded source image.

Summary: 4 cues, 2 migrated, 2 held for review, 2 with outline/shadow/AA, 1 animated, 0 decode failures.

Boundary verification: **PASS** (3 rounds, 48 checks, 0 failures).

## Cue 0001

- In/out: `00:00:01.000` .. `00:00:02.502` (`1000..2502 ms`)
- Source rect: `x=100 y=390 w=64 h=16`
- Flags: `text_not_recognized_no_ocr`
- ASS: `1000..2500 ms`, delta `+0/-2 ms`, colour `#FFFFFF`, alpha `0`
- Composite check: pixel match `true`, source deltas `+0/-2 ms`

![cue 0001 original](original/0001.png)

![cue 0001 frame](original/0001_frame.png)

![cue 0001 in](composite/0001_in.png)

![cue 0001 last](composite/0001_last.png)

![cue 0001 out](composite/0001_out.png)

## Cue 0002

- In/out: `00:00:03.000` .. `00:00:04.502` (`3000..4502 ms`)
- Source rect: `x=260 y=382 w=72 h=18`
- Flags: `text_not_recognized_no_ocr, outline_or_shadow_or_antialias`
- ASS: `3000..4500 ms`, delta `+0/-2 ms`, colour `#FFFFFF`, alpha `0`
- Composite check: pixel match `true`, source deltas `+0/-2 ms`

![cue 0002 original](original/0002.png)

![cue 0002 frame](original/0002_frame.png)

![cue 0002 in](composite/0002_in.png)

![cue 0002 last](composite/0002_last.png)

![cue 0002 out](composite/0002_out.png)

## Cue 0003

- In/out: `00:00:05.000` .. `00:00:07.002` (`5000..7002 ms`)
- Source rect: `x=430 y=370 w=56 h=16`
- Flags: `text_not_recognized_no_ocr, animation, outline_or_shadow_or_antialias`
- ASS: not emitted (unreliable or not representable)

![cue 0003 original](original/0003.png)

![cue 0003 frame](original/0003_frame.png)

![cue 0003 state_01 @ 0 ms](original/0003_state_01.png)

![cue 0003 state_01 frame @ 0 ms](original/0003_state_01_frame.png)

![cue 0003 state_02 @ 1001 ms](original/0003_state_02.png)

![cue 0003 state_02 frame @ 1001 ms](original/0003_state_02_frame.png)

## Cue 0004

- In/out: `00:00:08.000` .. `00:00:08.000` (`8000..8000 ms`)
- Source rect: `x=590 y=390 w=60 h=14`
- Flags: `text_not_recognized_no_ocr, timing_missing_stop`
- ASS: not emitted (unreliable or not representable)
- Notes: SPU has no stop-display command; end time is not guessed

![cue 0004 original](original/0004.png)

![cue 0004 frame](original/0004_frame.png)

