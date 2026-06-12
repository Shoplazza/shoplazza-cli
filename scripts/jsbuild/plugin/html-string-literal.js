'use strict';

// Turn any string into a source-code fragment that is a valid JS string literal.
// JSON.stringify produces a valid double-quoted JS string for any input (handles
// quotes, backslashes, newlines, control chars, unicode). Additionally escape
// U+2028/U+2029: legal in JSON but illegal in pre-ES2019 JS string literals -
// escape them explicitly for compatibility with all target runtimes.
var LS = '\u2028';
var PS = '\u2029';

function htmlToJsStringLiteral(s) {
  return JSON.stringify(String(s))
    .replace(new RegExp(LS, 'g'), '\\u2028')
    .replace(new RegExp(PS, 'g'), '\\u2029');
}

module.exports = { htmlToJsStringLiteral };
