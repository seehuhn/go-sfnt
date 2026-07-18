# varexpect fixtures

JSON files here are fontTools ground truth for `TestVarExpect`
(`../../varexpect_test.go`), independent of `Font.Instantiate`. Each records,
for one variable font and several pinned coordinate sets, per-glyph advance
widths, raw (un-flattened, un-implied-point) outline points, and selected
OS/2 metrics, computed by `fontTools.varLib.instancer`.

Source fonts (Junicode-VF.ttf, Elstob-VF.ttf) are SIL Open Font License
fonts fetched by `scripts/get-testfonts.sh`; they are not committed here.
The recorded `source_sha256` pins the exact font build each file was
generated against.

Regenerate with (requires `QUIRE_TESTFONTS` set and `fontTools` installed):

```sh
go generate ./go-sfnt/
```

or invoke `examples/scripts/gen-var-expect.py` directly; see the `//go:generate`
lines in `varexpect_test.go` for the exact arguments.
