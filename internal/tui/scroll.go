package tui

// wheelScrollLines is how many lines one mouse-wheel event nudges a list or
// viewport.
//
// A macOS trackpad flick (and Ghostty's translation of smooth scrolling into
// discrete wheel events) arrives as a rapid burst of these events. Multiplying
// each one by a large step makes the content lurch well past where the gesture
// meant to stop, which is what reads as "not smooth" next to the terminal's own
// scrollback. A small fixed step lets the burst accumulate naturally and keeps
// a notched mouse wheel usable.
const wheelScrollLines = 2
