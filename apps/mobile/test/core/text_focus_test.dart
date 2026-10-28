import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:lottery_mobile/core/input/text_focus.dart';

void main() {
  late TextEditingController text;

  Future<(FocusNode, List<bool>)> field(WidgetTester tester) async {
    final node = FocusNode();
    text = TextEditingController(text: 'secret-pass');
    addTearDown(node.dispose);
    addTearDown(text.dispose);
    final changes = <bool>[];
    node.addListener(() => changes.add(node.hasFocus));
    await tester.pumpWidget(
      MaterialApp(
        home: Scaffold(
          body: TextField(controller: text, focusNode: node, obscureText: true),
        ),
      ),
    );
    await tester.tap(find.byType(TextField));
    await tester.pump();
    changes.clear();
    return (node, changes);
  }

  testWidgets('on the web the focused field gives up its focus and takes it back, so its input is made anew', (
    tester,
  ) async {
    final (node, changes) = await field(tester);
    text.selection = const TextSelection.collapsed(offset: 6);
    reconnectFocusedTextField(web: true);
    await tester.pump(); // off, and back on after the frame
    text.selection = const TextSelection(baseOffset: 0, extentOffset: 11); // as the web does on gaining focus
    await tester.pump(); // the caret is put back
    await tester.pump();
    expect(changes, [false, true]);
    expect(node.hasFocus, isTrue);
    expect(text.selection, const TextSelection.collapsed(offset: 6), reason: 'the caret stays where it was');
    expect(text.text, 'secret-pass');
  });

  testWidgets('elsewhere, or with no text field focused, nothing moves', (tester) async {
    final (node, changes) = await field(tester);
    reconnectFocusedTextField(web: false);
    await tester.pump();
    expect(changes, isEmpty);
    node.unfocus();
    await tester.pump();
    changes.clear();
    reconnectFocusedTextField(web: true);
    await tester.pump();
    await tester.pump();
    expect(changes, isEmpty);
  });
}
