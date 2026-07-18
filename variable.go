// seehuhn.de/go/sfnt - a library for reading and writing font files
// Copyright (C) 2026  Jochen Voss <voss@seehuhn.de>
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.

package sfnt

// VariationAxis describes one axis of a variable font.
type VariationAxis struct {
	Tag               string
	Name              string
	Min, Default, Max float64
	Hidden            bool
}

// NamedInstance is a named instance of a variable font.
type NamedInstance struct {
	Name, PostScriptName string
	Coordinates          map[string]float64 // user scale, by axis tag
}

// IsVariable reports whether the font defines variation axes.
func (f *Font) IsVariable() bool {
	return f.Fvar != nil && len(f.Fvar.Axes) > 0
}

// VariationAxes returns the font's variation axes.
// It returns nil for a non-variable font.
func (f *Font) VariationAxes() []VariationAxis {
	if !f.IsVariable() {
		return nil
	}
	axes := make([]VariationAxis, len(f.Fvar.Axes))
	for i, a := range f.Fvar.Axes {
		axes[i] = VariationAxis{
			Tag:     a.Tag,
			Name:    a.Name,
			Min:     a.Min,
			Default: a.Default,
			Max:     a.Max,
			Hidden:  a.Hidden,
		}
	}
	return axes
}

// NamedInstances returns the font's named instances.
// It returns nil for a non-variable font or one without named instances.
func (f *Font) NamedInstances() []NamedInstance {
	if !f.IsVariable() {
		return nil
	}
	var res []NamedInstance
	for _, inst := range f.Fvar.Instances {
		coords := make(map[string]float64, len(f.Fvar.Axes))
		for i, a := range f.Fvar.Axes {
			if i < len(inst.Coordinates) {
				coords[a.Tag] = inst.Coordinates[i]
			}
		}
		res = append(res, NamedInstance{
			Name:           inst.Name,
			PostScriptName: inst.PostScriptName,
			Coordinates:    coords,
		})
	}
	return res
}
