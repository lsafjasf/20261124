# HDR->SDR double tone-mapping postmortem

Symptom: on bright shots the peak is not clipped, but skin tones look gray.

## 1. Back-solved transfer curves (buggy pipeline)

Each stage boundary is fitted against hypotheses; RMS ~ 0 identifies the true transfer.

```
frame 0 (normal-a, gain=1, HighlightProtect=false)
  scene -> exposure
    identity     gain=0.001  rms=0.000e+00  max=0.000e+00
    gain-only    gain=1  rms=1.879e-16  max=8.882e-16
    tonemap      gain=5.96254  rms=4.620e-01  max=1.610e+00
    srgb-encode  gain=0.3125  rms=5.943e-01  max=3.000e+00
    tonemap^2    gain=87.8581  rms=6.131e-01  max=1.671e+00
  exposure -> tonemap
    tonemap      gain=1  rms=4.977e-17  max=4.441e-16
    srgb-encode  gain=0.165723  rms=4.857e-02  max=1.661e-01
    tonemap^2    gain=1.51219  rms=8.891e-02  max=4.181e-01
    gain-only    gain=0.290599  rms=1.052e-01  max=1.946e-01
    identity     gain=0.001  rms=5.956e-01  max=3.000e+00
  tonemap -> look
    identity     gain=0.001  rms=0.000e+00  max=0.000e+00
    gain-only    gain=1  rms=6.009e-17  max=2.220e-16
    tonemap      gain=1.51019  rms=7.668e-02  max=3.416e-01
    srgb-encode  gain=0.238053  rms=1.112e-01  max=4.749e-01
    tonemap^2    gain=1.89356  rms=1.190e-01  max=5.581e-01
  look -> encode
    srgb-encode  gain=1  rms=8.583e-17  max=3.331e-16
    tonemap      gain=4.49577  rms=3.148e-02  max=6.874e-02
    tonemap^2    gain=12.9639  rms=7.180e-02  max=3.072e-01
    gain-only    gain=1.57279  rms=2.083e-01  max=5.728e-01
    identity     gain=0.001  rms=2.568e-01  max=2.871e-01
```
```
frame 1 (normal-a, gain=1, HighlightProtect=false)
  scene -> exposure
    identity     gain=0.001  rms=0.000e+00  max=0.000e+00
    gain-only    gain=1  rms=1.879e-16  max=8.882e-16
    tonemap      gain=5.96254  rms=4.620e-01  max=1.610e+00
    srgb-encode  gain=0.3125  rms=5.943e-01  max=3.000e+00
    tonemap^2    gain=87.8581  rms=6.131e-01  max=1.671e+00
  exposure -> tonemap
    tonemap      gain=1  rms=4.977e-17  max=4.441e-16
    srgb-encode  gain=0.165723  rms=4.857e-02  max=1.661e-01
    tonemap^2    gain=1.51219  rms=8.891e-02  max=4.181e-01
    gain-only    gain=0.290599  rms=1.052e-01  max=1.946e-01
    identity     gain=0.001  rms=5.956e-01  max=3.000e+00
  tonemap -> look
    identity     gain=0.001  rms=0.000e+00  max=0.000e+00
    gain-only    gain=1  rms=6.009e-17  max=2.220e-16
    tonemap      gain=1.51019  rms=7.668e-02  max=3.416e-01
    srgb-encode  gain=0.238053  rms=1.112e-01  max=4.749e-01
    tonemap^2    gain=1.89356  rms=1.190e-01  max=5.581e-01
  look -> encode
    srgb-encode  gain=1  rms=8.583e-17  max=3.331e-16
    tonemap      gain=4.49577  rms=3.148e-02  max=6.874e-02
    tonemap^2    gain=12.9639  rms=7.180e-02  max=3.072e-01
    gain-only    gain=1.57279  rms=2.083e-01  max=5.728e-01
    identity     gain=0.001  rms=2.568e-01  max=2.871e-01
```
```
frame 2 (bright, gain=0.25, HighlightProtect=true)
  scene -> exposure
    gain-only    gain=0.25  rms=1.879e-16  max=8.882e-16
    tonemap      gain=1.49064  rms=4.620e-01  max=1.610e+00
    srgb-encode  gain=0.078125  rms=5.943e-01  max=3.000e+00
    tonemap^2    gain=21.9645  rms=6.131e-01  max=1.671e+00
    identity     gain=0.001  rms=2.479e+00  max=1.200e+01
  exposure -> tonemap
    tonemap      gain=1  rms=4.977e-17  max=4.441e-16
    srgb-encode  gain=0.165723  rms=4.857e-02  max=1.661e-01
    tonemap^2    gain=1.51219  rms=8.891e-02  max=4.181e-01
    gain-only    gain=0.290599  rms=1.052e-01  max=1.946e-01
    identity     gain=0.001  rms=5.956e-01  max=3.000e+00
  tonemap -> look
    tonemap      gain=1  rms=3.384e-17  max=1.110e-16
    tonemap^2    gain=1.30092  rms=3.029e-02  max=1.373e-01
    srgb-encode  gain=0.1311  rms=3.394e-02  max=1.338e-01
    gain-only    gain=0.650476  rms=3.957e-02  max=1.192e-01
    identity     gain=0.001  rms=9.981e-02  max=4.688e-01
  look -> encode
    srgb-encode  gain=1  rms=6.253e-17  max=2.220e-16
    tonemap      gain=4.64569  rms=3.106e-02  max=6.585e-02
    tonemap^2    gain=11.0671  rms=4.191e-02  max=1.773e-01
    gain-only    gain=2.24356  rms=1.352e-01  max=4.363e-01
    identity     gain=0.001  rms=2.563e-01  max=2.871e-01
```
```
frame 3 (bright, gain=0.25, HighlightProtect=true)
  scene -> exposure
    gain-only    gain=0.25  rms=1.879e-16  max=8.882e-16
    tonemap      gain=1.49064  rms=4.620e-01  max=1.610e+00
    srgb-encode  gain=0.078125  rms=5.943e-01  max=3.000e+00
    tonemap^2    gain=21.9645  rms=6.131e-01  max=1.671e+00
    identity     gain=0.001  rms=2.479e+00  max=1.200e+01
  exposure -> tonemap
    tonemap      gain=1  rms=4.977e-17  max=4.441e-16
    srgb-encode  gain=0.165723  rms=4.857e-02  max=1.661e-01
    tonemap^2    gain=1.51219  rms=8.891e-02  max=4.181e-01
    gain-only    gain=0.290599  rms=1.052e-01  max=1.946e-01
    identity     gain=0.001  rms=5.956e-01  max=3.000e+00
  tonemap -> look
    tonemap      gain=1  rms=3.384e-17  max=1.110e-16
    tonemap^2    gain=1.30092  rms=3.029e-02  max=1.373e-01
    srgb-encode  gain=0.1311  rms=3.394e-02  max=1.338e-01
    gain-only    gain=0.650476  rms=3.957e-02  max=1.192e-01
    identity     gain=0.001  rms=9.981e-02  max=4.688e-01
  look -> encode
    srgb-encode  gain=1  rms=6.253e-17  max=2.220e-16
    tonemap      gain=4.64569  rms=3.106e-02  max=6.585e-02
    tonemap^2    gain=11.0671  rms=4.191e-02  max=1.773e-01
    gain-only    gain=2.24356  rms=1.352e-01  max=4.363e-01
    identity     gain=0.001  rms=2.563e-01  max=2.871e-01
```
```
frame 4 (bright, gain=0.25, HighlightProtect=true)
  scene -> exposure
    gain-only    gain=0.25  rms=1.879e-16  max=8.882e-16
    tonemap      gain=1.49064  rms=4.620e-01  max=1.610e+00
    srgb-encode  gain=0.078125  rms=5.943e-01  max=3.000e+00
    tonemap^2    gain=21.9645  rms=6.131e-01  max=1.671e+00
    identity     gain=0.001  rms=2.479e+00  max=1.200e+01
  exposure -> tonemap
    tonemap      gain=1  rms=4.977e-17  max=4.441e-16
    srgb-encode  gain=0.165723  rms=4.857e-02  max=1.661e-01
    tonemap^2    gain=1.51219  rms=8.891e-02  max=4.181e-01
    gain-only    gain=0.290599  rms=1.052e-01  max=1.946e-01
    identity     gain=0.001  rms=5.956e-01  max=3.000e+00
  tonemap -> look
    tonemap      gain=1  rms=3.384e-17  max=1.110e-16
    tonemap^2    gain=1.30092  rms=3.029e-02  max=1.373e-01
    srgb-encode  gain=0.1311  rms=3.394e-02  max=1.338e-01
    gain-only    gain=0.650476  rms=3.957e-02  max=1.192e-01
    identity     gain=0.001  rms=9.981e-02  max=4.688e-01
  look -> encode
    srgb-encode  gain=1  rms=6.253e-17  max=2.220e-16
    tonemap      gain=4.64569  rms=3.106e-02  max=6.585e-02
    tonemap^2    gain=11.0671  rms=4.191e-02  max=1.773e-01
    gain-only    gain=2.24356  rms=1.352e-01  max=4.363e-01
    identity     gain=0.001  rms=2.563e-01  max=2.871e-01
```
```
frame 5 (normal-b, gain=1, HighlightProtect=false)
  scene -> exposure
    identity     gain=0.001  rms=0.000e+00  max=0.000e+00
    gain-only    gain=1  rms=1.879e-16  max=8.882e-16
    tonemap      gain=5.96254  rms=4.620e-01  max=1.610e+00
    srgb-encode  gain=0.3125  rms=5.943e-01  max=3.000e+00
    tonemap^2    gain=87.8581  rms=6.131e-01  max=1.671e+00
  exposure -> tonemap
    tonemap      gain=1  rms=4.977e-17  max=4.441e-16
    srgb-encode  gain=0.165723  rms=4.857e-02  max=1.661e-01
    tonemap^2    gain=1.51219  rms=8.891e-02  max=4.181e-01
    gain-only    gain=0.290599  rms=1.052e-01  max=1.946e-01
    identity     gain=0.001  rms=5.956e-01  max=3.000e+00
  tonemap -> look
    identity     gain=0.001  rms=0.000e+00  max=0.000e+00
    gain-only    gain=1  rms=6.009e-17  max=2.220e-16
    tonemap      gain=1.51019  rms=7.668e-02  max=3.416e-01
    srgb-encode  gain=0.238053  rms=1.112e-01  max=4.749e-01
    tonemap^2    gain=1.89356  rms=1.190e-01  max=5.581e-01
  look -> encode
    srgb-encode  gain=1  rms=8.583e-17  max=3.331e-16
    tonemap      gain=4.49577  rms=3.148e-02  max=6.874e-02
    tonemap^2    gain=12.9639  rms=7.180e-02  max=3.072e-01
    gain-only    gain=1.57279  rms=2.083e-01  max=5.728e-01
    identity     gain=0.001  rms=2.568e-01  max=2.871e-01
```
## 2. Localization

- frame 2 (bright): unexpected tone map at stage boundary **tonemap->look**; HighlightProtect=true
- frame 3 (bright): unexpected tone map at stage boundary **tonemap->look**; HighlightProtect=true
- frame 4 (bright): unexpected tone map at stage boundary **tonemap->look**; HighlightProtect=true

Conclusion: the second tone map happens in module **look** (boundary tonemap->look), on frames **2,3,4**, exactly the frames where parameter **HighlightProtect=true** (auto-enabled for bright shots, KeyLight=4).

## 3. Why this is NOT an exposure problem

1. The exposure stage is purely linear on every frame: best `gain-only` fit has worst RMS = 1.879e-16 (machine precision). Exposure only scales; it cannot bend mid-tones.
2. The defect is nonlinear. Fitting the exposure->look composite on bright frame 2:

   ```
   tonemap^2    gain=1  rms=2.859e-17
   srgb-encode  gain=0.0808001  rms=1.883e-02
   tonemap      gain=0.532143  rms=6.457e-02
   gain-only    gain=0.170036  rms=1.044e-01
   identity     gain=0.001  rms=6.938e-01
   ```
   No gain-only or single-tonemap transfer explains the data; `tonemap^2` fits to machine precision.
3. Counterfactual gain sweep on bright frame 2 (single correct tone map, gain 0.01..100):
   - best achievable match to the BUGGY output: RMS = 4.487e-02 (no exposure value can produce the artifact);
   - best achievable match to the GOLDEN output: RMS = 7.814e-04 at gain = 0.2512, i.e. the auto exposure 1/KeyLight = 0.25 was already optimal.

## 4. Skin tone evidence (sRGB 8-bit)

| frame | shot | pipeline | skin RGB (8-bit) | chroma |
|---|---|---|---|---|
| 0 | normal-a | buggy | (152,132,121) | 0.205 |
| 0 | normal-a | fixed | (152,132,121) | 0.205 |
| 0 | normal-a | golden | (152,132,121) | 0.205 |
| 1 | normal-a | buggy | (152,132,121) | 0.205 |
| 1 | normal-a | fixed | (152,132,121) | 0.205 |
| 1 | normal-a | golden | (152,132,121) | 0.205 |
| 2 | bright | buggy | (136,121,112) | 0.172 |
| 2 | bright | fixed | (152,132,121) | 0.205 |
| 2 | bright | golden | (152,132,121) | 0.205 |
| 3 | bright | buggy | (136,121,112) | 0.172 |
| 3 | bright | fixed | (152,132,121) | 0.205 |
| 3 | bright | golden | (152,132,121) | 0.205 |
| 4 | bright | buggy | (136,121,112) | 0.172 |
| 4 | bright | fixed | (152,132,121) | 0.205 |
| 4 | bright | golden | (152,132,121) | 0.205 |
| 5 | normal-b | buggy | (152,132,121) | 0.205 |
| 5 | normal-b | fixed | (152,132,121) | 0.205 |
| 5 | normal-b | golden | (152,132,121) | 0.205 |

Buggy bright frames crush skin toward gray (chroma collapses); fixed frames match golden.

## 5. Fix verification vs single-map golden

| frame | shot | buggy RMS vs golden | fixed RMS vs golden |
|---|---|---|---|
| 0 | normal-a | 0.000e+00 | 0.000e+00 |
| 1 | normal-a | 0.000e+00 | 0.000e+00 |
| 2 | bright | 5.859e-02 | 0.000e+00 |
| 3 | bright | 5.859e-02 | 0.000e+00 |
| 4 | bright | 5.859e-02 | 0.000e+00 |
| 5 | normal-b | 0.000e+00 | 0.000e+00 |

Fix: the look module no longer applies the scene-referred tone curve; highlight rolloff stays solely in the tonemap stage (WhitePoint=4). HighlightProtect is now a no-op in look and kept only as recorded metadata.
