import 'package:flutter/foundation.dart';
import 'package:flutter/widgets.dart';

/// Reopens the keyboard connection of the text field that has focus, on the web.
///
/// The browser types into a hidden input that Flutter places over the focused field. When the window loses focus
/// (another app, a password manager, a browser prompt) that input can be removed while the field keeps its focus and
/// its caret, so typing, pasting and even clicking the field again do nothing. Moving the focus off and back makes
/// Flutter create the input anew; the caret and selection are put back where they were.
///
/// [web] is for tests; on the phone apps the platform keyboard never loses the field, so nothing is done.
void reconnectFocusedTextField({bool web = kIsWeb}) {
  if (!web) return;
  final node = FocusManager.instance.primaryFocus;
  final editable = node?.context?.findAncestorStateOfType<EditableTextState>();
  if (node == null || editable == null) return;
  final controller = editable.widget.controller;
  final before = controller.value;
  node.unfocus();
  WidgetsBinding.instance.addPostFrameCallback((_) {
    if (node.context == null || !node.canRequestFocus) return;
    node.requestFocus();
    // gaining focus on the web selects everything, and the next key would replace the whole password
    WidgetsBinding.instance.addPostFrameCallback((_) {
      if (node.hasFocus && controller.text == before.text && before.selection.isValid) {
        controller.selection = before.selection;
      }
    });
  });
}
