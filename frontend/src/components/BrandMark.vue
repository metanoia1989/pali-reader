<script setup>
// The site mark: a leaf lying on its side, closed into a ring, with one arc
// inside it for the eye.
//
// Drawn inline rather than loaded as an image so it takes `currentColor` and
// sits on any ground, and so the top bar, the auth pages and the favicon are
// the same drawing rather than three files that have to be kept in step.
// `frontend/public/favicon.svg` is this same geometry.
defineProps({
  size: { type: Number, default: 17 },
})
</script>

<template>
  <svg
    :width="size"
    :height="size"
    viewBox="0 0 64 64"
    fill="none"
    role="img"
    aria-label="巴利三藏阅读器"
  >
    <!-- The leaf is drawn upright and rotated: an eye is horizontal, and a leaf
       standing on end does not read as one. The ring is a filled band, not a
       hairline outline — at 16px an outline vanishes, a band this wide holds. -->
  <g transform="rotate(90 32 32)">
    <path d="M34 4C52.7 24.8 52.1 40.8 31.8 60C10.7 37.2 11.2 25.1 34 4ZM33.6 9C45.1 27.4 44.8 37.4 32 55C18 36.5 18.3 26.5 33.6 9Z" fill="currentColor" fill-rule="evenodd"/>
  </g>
  <!-- The eye line is an S laid on its side. A single upward bow reads as a
       mouth inside an almond; the S carries the iris, so the shape says "eye"
       rather than "almond with something in it", and the curve is shallow
       enough to survive at 16px. It is deliberately NOT rotated with the leaf:
       a single upright stroke inside a ring is a compass needle. -->
  <path d="M23.5 31.4C26.5 28.2 29.5 35.4 33 33.4C36 31.6 38 29.8 40.5 31.6" stroke="currentColor" stroke-width="5" stroke-linecap="round"/>
</svg>
</template>
