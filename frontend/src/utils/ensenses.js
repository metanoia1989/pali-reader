// Turning one dictionary entry into the rows the popup draws.
//
// Kept out of the component so it can be checked without a browser: the
// dictionary writes several glosses into one field, separated by line breaks
// and each carrying its own tag, and a row that comes out with the tag glued to
// the definition looks like a typo in the dictionary.
//
// ECDICT packs a word's meanings into one `def`, one per line, with the part of
// speech or the field at the head of each line:
//
//   n.  住处
//       [医] 住房
//
// The popup shows them as rows — tag in its own column, meaning in the other —
// because that is what makes them scannable, and because a reader who wants
// only the first line should not have to read past the medical gloss.

const LEADING_TAG = /^(?:([a-z]{1,6}\.)|(\[[^\]]{1,8}\]))\s*/i

export function expandSense(sense) {
  const out = []
  String(sense?.def ?? '')
    .split('\n')
    .forEach((raw, i) => {
      const line = raw.trim()
      if (!line) return
      let pos = ''
      let def = line
      const m = line.match(LEADING_TAG)
      if (m) {
        const rest = line.slice(m[0].length).trim()
        // A line that is nothing but a tag is not a meaning; leave it whole.
        if (rest) {
          pos = (m[1] || m[2]).trim()
          def = rest
        }
      }
      // The entry's own pos field belongs to its first line only.
      if (i === 0 && sense?.pos) pos = String(sense.pos).trim()
      out.push({ pos, def })
    })
  return out
}

// expandSenses flattens every sense of an entry into rows, without repeating
// one: ECDICT stores the same gloss under several tags often enough that the
// popup would otherwise print "n. 住处" twice.
export function expandSenses(senses) {
  const seen = new Set()
  const out = []
  for (const sense of senses || []) {
    for (const row of expandSense(sense)) {
      const key = row.pos + '\u0000' + row.def
      if (seen.has(key)) continue
      seen.add(key)
      out.push(row)
    }
  }
  return out
}
