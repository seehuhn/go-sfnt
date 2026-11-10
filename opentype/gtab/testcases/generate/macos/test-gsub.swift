// Copyright 2024 Jochen Voss <voss@seehuhn.de>
// SPDX-License-Identifier: BSD-3-Clause

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
