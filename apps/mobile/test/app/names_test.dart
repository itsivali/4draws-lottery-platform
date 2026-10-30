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

  // owner, 2026-10-30: a new account gives its first name, second name, then its national ID number (this replaces
  // 2026-10-30's "names come from the identity submission only")
  testWidgets('sign-up asks for the first name, second name, then the ID number', (t) async {
    await pumpApp(t, FakeBackend(), signedIn: false);
    await t.tap(find.text('Create account').first);
    await t.pumpAndSettle();
    final first = t.getTopLeft(find.widgetWithText(TextFormField, 'First name')).dy;
    final second = t.getTopLeft(find.widgetWithText(TextFormField, 'Second name')).dy;
    final id = t.getTopLeft(find.widgetWithText(TextFormField, 'National ID number')).dy;
    expect(first < second && second < id, isTrue, reason: 'in that order');
    await t.tap(find.text('Sign in').first);
    await t.pumpAndSettle();
    expect(find.widgetWithText(TextFormField, 'First name'), findsNothing, reason: 'signing in asks for none of it');
  });
}
