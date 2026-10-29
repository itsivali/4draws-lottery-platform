import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';

import '../support/app_harness.dart';
import '../support/fake_backend.dart';
import '../support/fonts.dart';

/// The player's names (owner, 2026-10-29): asked at sign-up, shown on the account screen above the number.
void main() {
  setUpAll(loadAppFonts);

  testWidgets('the account screen shows the names large and the number small', (t) async {
    await pumpApp(t, FakeBackend());
    await t.tap(find.text('Account').last);
    await t.pumpAndSettle();
    expect(find.text('Jane Wanjiru'), findsWidgets);
    expect(find.text('0712 345 678'), findsOneWidget);
  });

  testWidgets('an account without names shows the number and offers to add them; the names then save', (t) async {
    final backend = FakeBackend()
      ..firstName = null
      ..secondName = null;
    await pumpApp(t, backend);
    await t.tap(find.text('Account').last);
    await t.pumpAndSettle();
    expect(find.text('Add your first and second names'), findsOneWidget);
    await t.tap(find.text('Your name'));
    await t.pumpAndSettle();
    expect(find.text('Add your name'), findsOneWidget);
    await t.tap(find.widgetWithText(FilledButton, 'Add name'));
    await t.pumpAndSettle();
    expect(find.text('Enter your first name.'), findsOneWidget);
    await t.enterText(find.widgetWithText(TextFormField, 'First name'), 'Amina');
    await t.enterText(find.widgetWithText(TextFormField, 'Second name'), 'Otieno');
    await t.tap(find.widgetWithText(FilledButton, 'Add name'));
    await t.pumpAndSettle();
    expect(backend.firstName, 'Amina');
    expect(backend.secondName, 'Otieno');
    expect(find.text('Amina Otieno'), findsWidgets);
  });

  testWidgets('sign-up sends the names', (t) async {
    final backend = FakeBackend();
    await pumpApp(t, backend, signedIn: false);
    await t.tap(find.text('Create account').first);
    await t.pumpAndSettle();
    await t.enterText(find.widgetWithText(TextFormField, 'First name'), 'Jane');
    await t.enterText(find.widgetWithText(TextFormField, 'Second name'), 'Wanjiru');
    await t.enterText(find.widgetWithText(TextFormField, 'M-Pesa phone number'), '0712345678');
    await t.enterText(find.widgetWithText(TextFormField, 'Email address'), 'jane@example.com');
    await t.enterText(find.widgetWithText(TextFormField, 'Password'), 'correct horse battery');
    await t.tap(find.text('I am 18 or older'));
    await t.tap(find.widgetWithText(FilledButton, 'Create account'));
    await t.pumpAndSettle();
    final signup = backend.requests.lastWhere((r) => r.url.path == '/v1/auth/signup');
    expect(signup.body, contains('"first_name":"Jane"'));
    expect(signup.body, contains('"second_name":"Wanjiru"'));
  });
}
