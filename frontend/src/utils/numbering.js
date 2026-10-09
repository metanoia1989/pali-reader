// Numbers in the reading surfaces.
//
// Two rules, both learned by looking at how tipitaka-pali-reader handles the
// same corpus:
//
// 1. A table of contents shows the section *name* and nothing else. The canon
//    numbers its own chapters and suttas ("1. brahmajālasuttaṃ", "2. sāmaññaphalasuttam"),
//    and adding a § in front of that produces "§ 149 2. sāmaññaphalasuttam",
//    which is two numbers where the reader wants one. Hierarchy is carried by
//    indentation and type size instead — exactly what that app does.
//
// 2. In the running text there is no such duplication: the source's paragraph
//    number was cut out at import time and the reader draws its own § in the
//    margin. That § stays, because it is what a citation uses.
// Table-of-contents levels, in the source's own vocabulary:
//   chapter · title · subhead · subsubhead
// Levels 1 and 2 are the piṭaka and the book — the reader is already inside
// them, so they do not belong in a list of the book's contents.
export const TOC_MIN_LEVEL = 3

// tocStyle returns the indentation and type size for a heading level.
//
// **Indentation alone carries the hierarchy.** Every level is the same size —
// shrinking the deeper ones is what made them unreadable in the first place —
// and now the same weight too. Weight was tried as one last distinction and
// rejected: the rail's own tree sits at 400, so a chapter list at 500 read as
// the boldest thing in the column rather than as the deepest thing in it, and
// which level got 500 was arbitrary anyway.
export function tocStyle(level) {
  switch (level) {
    case 3:
      return { indent: 0, size: 15.5 }
    case 4:
      return { indent: 16, size: 15.5 }
    case 5:
      return { indent: 32, size: 15.5 }
    default:
      return { indent: 48, size: 15.5 }
  }
}

export function tocEntries(toc) {
  return (toc || []).filter((t) => (t.level || 9) >= TOC_MIN_LEVEL)
}
