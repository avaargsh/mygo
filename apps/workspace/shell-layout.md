# Workspace shell layout

The shell owns six stable surfaces. Providers render only their content
inside the active pane; the shell does not duplicate their lifecycle logic.

- **Activity Bar:** switch between Explorer and Sessions, or hide the sidebar.
  Switching or hiding the sidebar does not change the active tab.
- **Workspace Explorer:** launch content and change appearance. Content
  launchers come from the same command registry used by Commands.
- **Tab Bar:** every pane retains its own tab list, selection and close
  controls. Closing a tab is not a session stop.
- **Content Area:** retain the recursive split layout and put pane operations
  immediately above it.
- **Session Manager:** use the existing explicit controls for hidden browser,
  terminal, file access and chat state. Save a pending note without restoring
  its tab. Explorer and Sessions share a sidebar so content retains space.
- **Status Bar:** show pane and tab counts, save state, unsaved note
  indication, running chats and the latest message.

This first shell revision does not change the snapshot schema, persist
sidebar preference, install a new design dependency, or start background
work. The Activity Bar remains available when the sidebar is hidden.
