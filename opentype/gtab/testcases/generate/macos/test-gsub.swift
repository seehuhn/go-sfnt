// seehuhn.de/go/sfnt - a library for reading and writing font files
// Copyright (C) 2024  Jochen Voss <voss@seehuhn.de>
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

import CoreText
import Foundation

// Usage
guard CommandLine.arguments.count == 3 else {
  print("usage: \(CommandLine.arguments[0]) <font-file-path> <input-string>")
  exit(EXIT_FAILURE)
}

let fontFilePath = CommandLine.arguments[1]
let inputString = CommandLine.arguments[2]

enum FontLoadingError: Error {
  case urlCreationFailed
  case dataProviderCreationFailed
  case fontCreationFailed
}

func loadFontFromFile(atPath path: String) throws -> CTFont {
  let cfPath = path as CFString
  guard
    let fontURL = CFURLCreateWithFileSystemPath(
      kCFAllocatorDefault, cfPath, CFURLPathStyle.cfurlposixPathStyle, false)
  else {
    throw FontLoadingError.urlCreationFailed
  }

  guard let fontDataProvider = CGDataProvider(url: fontURL) else {
    throw FontLoadingError.dataProviderCreationFailed
  }

  guard let fontRef = CGFont(fontDataProvider) else {
    throw FontLoadingError.fontCreationFailed
  }

  let ctFont = CTFontCreateWithGraphicsFont(fontRef, 0, nil, nil)

  return ctFont
}

// load the font
let font = try loadFontFromFile(atPath: fontFilePath)

// make a map for mapping glyph IDs to Unicode code points
let numGlyphs = CTFontGetGlyphCount(font)
var glyphToUnicodeMap: [UniChar] = Array(repeating: 0, count: Int(numGlyphs))
for unicodeChar: UniChar in 0x0001...0xFFFF {
  var glyph: CGGlyph = 0
  if CTFontGetGlyphsForCharacters(font, [unicodeChar], &glyph, 1) {
    if glyph != 0 && glyphToUnicodeMap[Int(glyph)] == 0 {
      glyphToUnicodeMap[Int(glyph)] = unicodeChar
    }
  }
}

let attributedString = NSAttributedString(
  string: inputString, attributes: [kCTFontAttributeName as NSAttributedString.Key: font])
let line = CTLineCreateWithAttributedString(attributedString)

// Get the glyph runs
let glyphRuns = CTLineGetGlyphRuns(line) as! [CTRun]

// Iterate through the glyph runs
let res = NSMutableString()
for run in glyphRuns {
  let glyphCount = CTRunGetGlyphCount(run)
  var glyphs = [CGGlyph](repeating: 0, count: glyphCount)
  CTRunGetGlyphs(run, CFRange(location: 0, length: 0), &glyphs)

  let unicodeCharacters = glyphs.compactMap { glyphID -> UniChar? in
    guard glyphID < glyphToUnicodeMap.count else { return nil }
    return glyphToUnicodeMap[Int(glyphID)]
  }

  // Create a String from the Unicode characters
  let string = String(utf16CodeUnits: unicodeCharacters, count: unicodeCharacters.count)
  res.append(string)
}
print(res)
