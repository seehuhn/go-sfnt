// seehuhn.de/go/sfnt - a library for reading and writing font files
// Copyright (C) 2023  Jochen Voss <voss@seehuhn.de>
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

package type1

func obfuscateCharstring(plain []byte, iv []byte) []byte {
	var R uint16 = 4330
	var c1 uint16 = 52845
	var c2 uint16 = 22719
	cipher := make([]byte, len(iv)+len(plain))
	copy(cipher, iv)
	copy(cipher[len(iv):], plain)
	for i, c := range cipher {
		c = c ^ byte(R>>8)
		R = (uint16(c)+R)*c1 + c2
		cipher[i] = c
	}
	return cipher
}

func deobfuscateCharstring(cipher []byte, n int) []byte {
	var R uint16 = 4330
	var c1 uint16 = 52845
	var c2 uint16 = 22719
	plain := make([]byte, 0, len(cipher)-n)
	for i, cipher := range cipher {
		if i >= n {
			plain = append(plain, cipher^byte(R>>8))
		}
		R = (uint16(cipher)+R)*c1 + c2
	}
	return plain
}
