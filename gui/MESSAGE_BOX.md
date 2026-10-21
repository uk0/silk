# Modal message boxes

`ShowMessageBox(parent, icon, title, message, buttons)` remains a synchronous
UI-thread modal helper and returns the selected button's exact string. It now
centers on the owning form rather than a narrow child panel. Details are wrapped,
read-only, selectable and vertically scrollable; the full message is retained for
Select All / Copy, including long file paths, diagnostic codes and recovery hints.
Buttons use the same right-aligned sizing and spacing as `Dialog`.

- Initial focus is assigned to details after native activation, without stealing
  the owner's saved focus while constructing the message.
- Enter accepts a single acknowledgment, including when details have focus.
  A Tab-focused button keeps its own action.
- For multi-button dialogs Cancel/No is preferred; known OK/Save/Yes roles follow.
  Multiple unknown custom buttons have no implicit affirmative default.
- `MessageBox.SetDefaultButton(name)` explicitly chooses a named button. Disabled
  or hidden buttons are not activated. Repeated Enter does not resubmit.
- Escape returns an available Cancel button's exact name, otherwise an empty
  string. Native close and recovered modal failure do not imply acceptance.
- `MessageBox.LastError()` reports recovered modal errors (`SILK-GUI001`), and is
  reset on reuse. The helper still returns a string for compatibility; callers
  needing error details can construct `NewMessageBox` explicitly.

Both native backends now isolate application-level shortcuts while a modal form
(or its popup/child window) is receiving keys. Local editing/selection/copy and
focused widget shortcut overrides still work; global IDE Open/Save/Undo/Quit do
not operate on documents behind the modal. Ordinary nonmodal bindings retain
priority. This does not replace host coordination for programmatic callbacks.

Regression evidence includes a real Windows rename-conflict dialog: Enter was
inert, application Save/Open continued behind it, and closing the owner timed
out. Clicking the acknowledgment then allowed normal exit. The pre-fix screenshot
and failing logical tests are preserved; verification requires rerunning that
native flow, not merely a successful build.
