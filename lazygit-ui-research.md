# How lazygit builds its interactive terminal UI

Research date: 2026-09-07. Source snapshot: [`c07f4d381b90419583b7ce04f87379654d983ebc`](https://github.com/jesseduffield/lazygit/tree/c07f4d381b90419583b7ce04f87379654d983ebc), committed 2026-09-05. This document describes that source snapshot; older releases can have different libraries, defaults, and behavior. Findings come from source inspection, not a live terminal interaction test.

Lazygit combines a **terminal cell renderer**, a **recursive layout engine**, and a **context-based interaction system**. The boxes are framed terminal views. Their dimensions depend on terminal size, focus, configuration, and screen mode. Keyboard and mouse handlers update the same application state, which then drives selection highlights, content, and layout.

## 1. The implementation stack

| Layer | Implementation | Responsibility |
| --- | --- | --- |
| Terminal backend | `github.com/gdamore/tcell/v3`, version `v3.4.2` in this snapshot | Terminal screen, input events, cell styles, and display updates |
| View toolkit | Lazygit's own `pkg/gocui` | Rectangular views, borders, titles, scrolling, editing, event routing, and drawing |
| Layout solver | `github.com/jesseduffield/lazycore/pkg/boxlayout` | Divide available terminal cells into named rectangles |
| Application layout | `WindowArrangementHelper` and `layout()` | Choose panel arrangement and apply rectangles to views |
| Interaction | Context manager and controllers | Focus, selected items, navigation, commands, and modal behavior |

The current application imports `github.com/jesseduffield/lazygit/pkg/gocui`. This matters when researching examples: an older external gocui package may not contain the mouse, rendering, or keyboard behavior described here. Sources: [dependencies](https://github.com/jesseduffield/lazygit/blob/c07f4d381b90419583b7ce04f87379654d983ebc/go.mod), [terminal driver](https://github.com/jesseduffield/lazygit/blob/c07f4d381b90419583b7ce04f87379654d983ebc/pkg/gocui/tcell_driver.go), [application layout](https://github.com/jesseduffield/lazygit/blob/c07f4d381b90419583b7ce04f87379654d983ebc/pkg/gui/layout.go).

## 2. How the boxed panels are drawn

A gocui `View` has coordinates, content, scrolling origin, a cursor, visibility, frame settings, and title information. The renderer draws frame edges and corners with terminal glyphs, draws titles and tabs into the frame, and draws content inside it. Focus affects frame styling and selection appearance. The terminal backend has ASCII fallbacks for border characters.

Relevant functions in `pkg/gocui/gui.go` include `SetView`, `drawFrameEdges`, `drawFrameCorners`, `drawTitle`, `draw`, and `flush`. A full flush runs layout managers, draws the visible views, and calls `Screen.Show()`. There is also a content-only redraw path. Sources: [view renderer](https://github.com/jesseduffield/lazygit/blob/c07f4d381b90419583b7ce04f87379654d983ebc/pkg/gocui/gui.go), [view data](https://github.com/jesseduffield/lazygit/blob/c07f4d381b90419583b7ce04f87379654d983ebc/pkg/gocui/view.go), [glyph fallbacks](https://github.com/jesseduffield/lazygit/blob/c07f4d381b90419583b7ce04f87379654d983ebc/pkg/gocui/tcell_driver.go).

Three concepts keep geometry separate from interaction:

- **Window:** a named layout slot, such as the area occupied by a side panel.
- **View:** the rendered surface placed in that slot; tabbed views can share a window.
- **Context:** the behavior and state associated with the content, including controllers, selection, and focus callbacks.

Activating a context assigns it to its window, raises the relevant view, sets the input view, and runs its focus handler. Sources: [window helper](https://github.com/jesseduffield/lazygit/blob/c07f4d381b90419583b7ce04f87379654d983ebc/pkg/gui/controllers/helpers/window_helper.go), [context activation](https://github.com/jesseduffield/lazygit/blob/c07f4d381b90419583b7ce04f87379654d983ebc/pkg/gui/context.go).

## 3. The layout engine

`boxlayout.Box` forms a tree. Each box can specify a window name, children, an orientation, a fixed `Size`, or a flexible `Weight`. Conditional callbacks can select children or orientation using the space assigned to that box.

The solver reserves fixed sizes first, distributes remaining space by weight, and distributes integer remainders so the results fit the terminal cell grid. For example, after fixed reservations, weights 1 and 2 receive approximately one-third and two-thirds of the remaining space.

**Naming detail:** `ROW` means children form stacked rows, dividing height. `COLUMN` means children form adjacent columns, dividing width. This differs from the direction naming many developers expect from CSS flexbox.

`ArrangeWindows` recursively produces a map of window names to inclusive `X0`, `Y0`, `X1`, `Y1` bounds. Source: [complete layout solver](https://github.com/jesseduffield/lazygit/blob/c07f4d381b90419583b7ce04f87379654d983ebc/vendor/github.com/jesseduffield/lazycore/pkg/boxlayout/boxlayout.go).

Conceptually, the normal layout looks like this:

```text
Screen
├── Working area
│   ├── Side section: vertically stacked windows
│   │   ├── Status
│   │   ├── Files
│   │   ├── Branches / related tabs
│   │   ├── Commits / related tabs
│   │   └── Stash
│   └── Main section
│       ├── Main content, optionally split with secondary content
│       └── Optional command log
└── Bottom information / shortcuts / search area
```

The side section's default width ratio is `0.3333`; the application converts this to integer section weights. Source: [layout tree and section weights](https://github.com/jesseduffield/lazygit/blob/c07f4d381b90419583b7ce04f87379654d983ebc/pkg/gui/controllers/helpers/window_arrangement_helper.go).

## 4. Responsive collapse and expansion

### Automatic collapse when height is limited

`sidePanelChildren` examines the **height allocated to the side section**, which can differ from total terminal height. For the default five side windows:

| Allocated side height | Behavior in normal screen mode |
| --- | --- |
| At least 28 rows | Use normal fixed and weighted sizes |
| 21–27 rows | Inactive side windows get 3 rows each; focused window takes the remainder |
| Below 21 rows | Inactive side windows get 1 row each; focused window takes the remainder |

With fewer windows, thresholds scale down: `min(28, 28 * count / 5)` and `min(21, 21 * count / 5)`, using integer division. Extra windows do not raise these thresholds.

At normal height, the status view gets 3 rows. An unfocused stash view also gets 3; a focused stash can grow. Other panels share flexible space. These status/stash rules follow the active tab, so a differently grouped tab does not inherit the wrong fixed size.

With `expandFocusedSidePanel: true`, the focused flexible panel gets `expandedSidePanelWeight`, default 2. This is proportional expansion, not a fixed number of extra rows. A separate `shrinkSidePanelsToContent` option caps panels that fit their content and redistributes remaining room. Both options default to false. Source: [collapse, accordion, and content sizing algorithms](https://github.com/jesseduffield/lazygit/blob/c07f4d381b90419583b7ce04f87379654d983ebc/pkg/gui/controllers/helpers/window_arrangement_helper.go).

### Portrait arrangement and main content splits

In automatic portrait mode, the side section moves above the main section when terminal width is **at most 84 columns** and height is **at least 46 rows** by default. Configuration can force or disable this orientation.

The main/secondary content split has a separate rule: in automatic split mode, it stacks when terminal width is below 200 and height exceeds 30; otherwise it places the views side by side. The configuration values are `vertical` for stacked content and `horizontal` for side-by-side content in this implementation.

These decisions live in `shouldUsePortraitMode` and `splitMainPanelSideBySide`. Source: [orientation rules](https://github.com/jesseduffield/lazygit/blob/c07f4d381b90419583b7ce04f87379654d983ebc/pkg/gui/controllers/helpers/window_arrangement_helper.go).

### Explicit screen modes

`+` and `_` advance and reverse the screen modes. Their layout meaning depends on focus:

| Mode | Side window focused | Main/secondary window focused |
| --- | --- | --- |
| Normal | Regular side stack and main section | Regular side stack and main section |
| Half | Only the active side window gets side-section space; left arrangement gives it equal section weight to main | Side section receives zero weight |
| Full | Main section receives zero weight; active side window fills the working area | Side section receives zero weight; focused main or secondary content is shown alone |

For half mode with `enlargedSideViewLocation: top`, the side section is above main and receives one-third of the combined section space. Thus “half” is a mode name, not a universal 50% sizing guarantee. Sources: [section weights and screen-mode layout](https://github.com/jesseduffield/lazygit/blob/c07f4d381b90419583b7ce04f87379654d983ebc/pkg/gui/controllers/helpers/window_arrangement_helper.go), [screen-mode key handlers](https://github.com/jesseduffield/lazygit/blob/c07f4d381b90419583b7ce04f87379654d983ebc/pkg/gui/controllers/global_controller.go).

### Applying resize results

`layout()` reads the terminal size, requests window dimensions, applies them through `SetView`, updates visibility and scroll bounds, and rerenders contexts that depend on changed width or height. It also resizes popups and loads more main-view lines when needed. Extremely small screens show a limit view: width below 10 or height below `max(9, sideWindowCount + 4)`, with a higher height floor of 11 for a filterable menu. Source: [resize and minimum-size handling](https://github.com/jesseduffield/lazygit/blob/c07f4d381b90419583b7ce04f87379654d983ebc/pkg/gui/layout.go).

## 5. Cursor, selection, and mouse interaction

“Cursor” covers three separate states: the mouse pointer, the selected list item, and the text insertion cursor. `ContextMgr.Activate` shows the terminal cursor for editable, unmasked views. Lists maintain an application selection index and render a highlight instead. Sources: [context activation](https://github.com/jesseduffield/lazygit/blob/c07f4d381b90419583b7ce04f87379654d983ebc/pkg/gui/context.go), [list cursor](https://github.com/jesseduffield/lazygit/blob/c07f4d381b90419583b7ce04f87379654d983ebc/pkg/gui/context/traits/list_cursor.go).

Mouse routing works in stages:

1. Locate the visible view under the pointer, or use the view that captured an ongoing drag.
2. Convert screen coordinates into viewport coordinates, subtracting the view position and frame offset.
3. Add the view's scrolling origin to obtain content coordinates.
4. Route a title-row click to tab handling, or dispatch the view's mouse handler.

For vertical coordinates, the core conversion is:

```text
viewportY = mouseY - viewTop - 1
contentY  = viewportY + scrollOriginY
modelIndex = context.ViewIndexToModelIndex(contentY)
```

The final mapping matters because rendered rows can differ from model rows. A click selects the item and focuses its context. A supported double-click action runs only when the context was already focused. Dragging can extend a selection and autoscroll at the viewport edge; mouse capture keeps the gesture attached to its starting view. Sources: [coordinate conversion and mouse capture](https://github.com/jesseduffield/lazygit/blob/c07f4d381b90419583b7ce04f87379654d983ebc/pkg/gocui/gui.go), [list click and drag handlers](https://github.com/jesseduffield/lazygit/blob/c07f4d381b90419583b7ce04f87379654d983ebc/pkg/gui/controllers/list_controller.go).

Mouse-wheel handlers scroll the target views. Popup-related mouse filtering can reject interactions with inactive views. These mechanisms explain clicking, scrolling, and selection dragging; the panel sizing behavior traced above comes from layout rules and configuration, rather than a splitter-drag algorithm. Sources: [mouse bindings](https://github.com/jesseduffield/lazygit/blob/c07f4d381b90419583b7ce04f87379654d983ebc/pkg/gui/keybindings.go), [event dispatch](https://github.com/jesseduffield/lazygit/blob/c07f4d381b90419583b7ce04f87379654d983ebc/pkg/gocui/gui.go).

## 6. Keyboard navigation and focus

Common default keys in this snapshot:

| Intent | Keys |
| --- | --- |
| Previous/next list item | Up/Down or `k`/`j` |
| Previous/next side panel | Left/Right, `h`/`l`, Shift+Tab/Tab |
| Jump to a side panel by position | `1`–`5` |
| Focus main view | `0` |
| Previous/next tab within a window | `[` / `]` |
| Previous/next list page | `,` / `.` |
| Top/bottom of list | `<` / `>`, Home/End |
| Scroll main content | PageUp/PageDown, `K`/`J`, Ctrl+U/Ctrl+D |
| Start search/filter interaction | `/` |
| Toggle range selection | `v`; Shift+Up/Down extends a nonsticky range |
| Advance/reverse screen mode | `+` / `_` |
| Options menu | `?` |

These are defaults and context-dependent actions, not promises that every view treats every key identically. Source: [configured default bindings](https://github.com/jesseduffield/lazygit/blob/c07f4d381b90419583b7ce04f87379654d983ebc/pkg/config/user_config.go).

Side-panel navigation follows the configured window order and wraps at the ends. It does not compute the nearest rectangle in the direction of an arrow. Numeric jump keys are assigned by panel position. With `switchTabsWithPanelJumpKeys: true`, pressing the current panel's jump key again advances its tab; this option defaults to false. Sources: [side navigation](https://github.com/jesseduffield/lazygit/blob/c07f4d381b90419583b7ce04f87379654d983ebc/pkg/gui/controllers/side_window_controller.go), [panel jump behavior](https://github.com/jesseduffield/lazygit/blob/c07f4d381b90419583b7ce04f87379654d983ebc/pkg/gui/controllers/jump_to_side_window_controller.go).

List movement updates a selected model index, keeps it within bounds, and adjusts the viewport through focus handling. Selection and scrolling remain distinct state. Sources: [list navigation controller](https://github.com/jesseduffield/lazygit/blob/c07f4d381b90419583b7ce04f87379654d983ebc/pkg/gui/controllers/list_controller.go), [list focus and viewport updates](https://github.com/jesseduffield/lazygit/blob/c07f4d381b90419583b7ce04f87379654d983ebc/pkg/gui/context/list_context_trait.go).

The context stack handles transitions such as side list → main detail → popup. Switching to a side context replaces the stack; entering a main context replaces other main contexts while preserving underlying non-main contexts. Popping restores and activates the previous context. Activation updates input focus, visibility, title, cursor visibility, highlights, and content callbacks. This connects keyboard focus to responsive expansion. Source: [context stack implementation](https://github.com/jesseduffield/lazygit/blob/c07f4d381b90419583b7ce04f87379654d983ebc/pkg/gui/context.go).

## 7. How shortcuts change with context

Controllers return binding objects containing keys, handlers, descriptions, scope, and optional disabled-state checks. The application collects bindings from contexts, assigns their view names, adds global bindings, and registers them with gocui. Custom-command bindings are prepended to give them precedence within the applicable dispatch rules. Search/filter input has a special path that registers only that context's bindings. Source: [binding construction and registration](https://github.com/jesseduffield/lazygit/blob/c07f4d381b90419583b7ce04f87379654d983ebc/pkg/gui/keybindings.go).

For ordinary key events, dispatch prefers the matching view binding, then a recorded parent-view binding, then editable text handling, then a recorded global binding. Search navigation has special interception. A handler can report that it did not handle the key, allowing the dispatcher's fallback behavior. Source: [`execKeybindings`](https://github.com/jesseduffield/lazygit/blob/c07f4d381b90419583b7ce04f87379654d983ebc/pkg/gocui/gui.go).

This means a key can invoke different commands in different views while common navigation remains reusable. Disabled checks and popup guards prevent actions in inappropriate states. The bottom shortcut bar is generated from current-context and global binding metadata, filters out disabled or undisplayed bindings, and truncates to available width. Sources: [command guards](https://github.com/jesseduffield/lazygit/blob/c07f4d381b90419583b7ce04f87379654d983ebc/pkg/gui/keybindings.go), [shortcut-bar generation](https://github.com/jesseduffield/lazygit/blob/c07f4d381b90419583b7ce04f87379654d983ebc/pkg/gui/options_map.go).

## 8. Configuration to experiment with

This example intentionally enables optional behaviors; it is not a dump of defaults. Check availability against the lazygit version you install.

```yaml
gui:
  border: rounded
  mouseEvents: true
  sidePanelWidth: 0.3333
  expandFocusedSidePanel: true
  expandedSidePanelWeight: 2
  shrinkSidePanelsToContent: true
  screenMode: normal
  enlargedSideViewLocation: left
  portraitMode: auto
  portraitModeAutoMaxWidth: 84
  portraitModeAutoMinHeight: 46
  mainPanelSplitMode: flexible
  switchTabsWithPanelJumpKeys: true
```

Sources: [configuration fields and defaults](https://github.com/jesseduffield/lazygit/blob/c07f4d381b90419583b7ce04f87379654d983ebc/pkg/config/user_config.go), [configuration documentation](https://github.com/jesseduffield/lazygit/blob/c07f4d381b90419583b7ce04f87379654d983ebc/docs-master/Config.md).

## 9. Applying the design to lazyleet

The following is an architectural recommendation derived from the research, not a description of existing lazyleet code.

Use a pure layout function whose inputs include terminal dimensions, active panel, screen mode, and optional content sizes. Return named rectangles. Keep panel geometry independent from selected problem, scroll position, editor cursor, and active tab.

For a problem-solving interface, a comparable arrangement could put Problems, Examples, and Runs in a side section, with Statement, Code, and Output as main views or tabs. Limited height can collapse inactive side panels while preserving their labels. Explicit focus modes can give the editor or output the working area.

A practical implementation sequence:

1. Build framed views and a fixed/weighted rectangle solver.
2. Introduce context state and semantic actions such as `NextItem`, `NextPanel`, `NextTab`, and `CycleScreenMode`.
3. Bind both keys and mouse events to those actions or shared selection/focus operations.
4. Add collapse thresholds and focus-dependent weights to the layout function.
5. Convert pointer positions through viewport and scroll coordinates before selecting model items.
6. Generate shortcut hints from the same binding definitions used for dispatch.
7. Verify resize boundaries, empty lists, scrolled clicks, tab changes, and popup focus restoration.

For implementation reference, start with the small [boxlayout solver](https://github.com/jesseduffield/lazygit/blob/c07f4d381b90419583b7ce04f87379654d983ebc/vendor/github.com/jesseduffield/lazycore/pkg/boxlayout/boxlayout.go), then read [window arrangement](https://github.com/jesseduffield/lazygit/blob/c07f4d381b90419583b7ce04f87379654d983ebc/pkg/gui/controllers/helpers/window_arrangement_helper.go), [context management](https://github.com/jesseduffield/lazygit/blob/c07f4d381b90419583b7ce04f87379654d983ebc/pkg/gui/context.go), and [list interactions](https://github.com/jesseduffield/lazygit/blob/c07f4d381b90419583b7ce04f87379654d983ebc/pkg/gui/controllers/list_controller.go). These expose the reusable design most directly.
