import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';

import '../support/app_harness.dart';
import '../support/fake_backend.dart';
import '../support/fonts.dart';

/// The player's name (owner, 2026-10-30): the one on their identity submission, shown on the account screen above
/// the number; nothing is asked at sign-up.
void main() {
  setUpAll(loadAppFonts);

  testWidgets('the account screen shows the name large and the number small', (t) async {
    await pumpApp(t, FakeBackend());
    await t.tap(find.text('Account').last);
    await t.pumpAndSettle();
    expect(find.text('Jane Wanjiru'), findsOneWidget);
    expect(find.text('0712 345 678'), findsOneWidget);
  });

  testWidgets('before any identity submission the number stands in for the name', (t) async {
    await pumpApp(
      t,
      FakeBackend()
        ..firstName = null
        ..secondName = null,
    );
    await t.tap(find.text('Account').last);
    await t.pumpAndSettle();
    expect(find.text('0712 345 678'), findsOneWidget);
    expect(find.text('Your name'), findsNothing);
  });

  testWidgets('sign-up asks for no name', (t) async {
    await pumpApp(t, FakeBackend(), signedIn: false);
    await t.tap(find.text('Create account').first);
    await t.pumpAndSettle();
    expect(find.widgetWithText(TextFormField, 'First name'), findsNothing);
  });
}
