import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';

import 'package:lottery_mobile/app/router.dart';

import '../support/app_harness.dart';
import '../support/fake_backend.dart';
import '../support/fonts.dart';
import '../support/journeys.dart';

/// The player's name (owner, 2026-10-30): the one on their identity submission, else the one they declared, shown on
/// the account screen above the number. An account without one adds it from the account (owner, 2026-10-31).
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
    expect(find.text('Add your names'), findsNothing, reason: 'the account has a declaration; nothing to add');
  });

  // owner, 2026-10-31: players whose accounts have no names fill them in from the account
  testWidgets('an account without names can add them from the account screen, and Home greets them afterwards', (
    t,
  ) async {
    final backend = FakeBackend()
      ..firstName = null
      ..secondName = null
      ..declared = null
      ..identityStatus = 'none';
    await pumpApp(t, backend);
    expect(find.text('Karibu'), findsOneWidget);
    expect(find.text('Add your names and ID number'), findsOneWidget, reason: 'Home asks until they are given');
    await t.tap(find.text('Account').last);
    await t.pumpAndSettle();
    expect(find.text('Add your names'), findsOneWidget, reason: 'the hint stands where the name will');
    expect(find.text('0712 345 678'), findsOneWidget);
    await t.tap(find.text('Add your names'));
    await t.pumpAndSettle();
    expect(find.text('Your details'), findsWidgets);
    await fillIdentity(t, first: 'Grace', second: 'Wanjiru', id: '13579246');
    await tapVisible(t, find.widgetWithText(FilledButton, 'Save my details'));
    await t.pumpAndSettle();
    final sent = backend.requests.lastWhere((r) => r.method == 'POST' && r.url.path == '/v1/account/identity');
    expect(sent.body, contains('"first_name":"Grace"'));
    expect(sent.body, contains('"national_id":"13579246"'));
    expect(sent.headers['Idempotency-Key'], isNotEmpty);
    expect(find.text('Grace Wanjiru'), findsOneWidget, reason: 'back on the account screen with the name');
    expect(find.text('Add your names'), findsNothing);
    await t.tap(find.text('Your details'));
    await t.pumpAndSettle();
    expect(find.text('•••••246'), findsOneWidget, reason: 'the ID number is shown masked');
    expect(find.text('Checked before your first withdrawal'), findsOneWidget);
    expect(find.widgetWithText(FilledButton, 'Save my details'), findsNothing, reason: 'given once');
    await t.tap(find.text('Home').last);
    await t.pumpAndSettle();
    expect(find.text('Karibu, Grace'), findsOneWidget);
    expect(find.text('Add your names and ID number'), findsNothing);
  });

  testWidgets('a second declaration is refused with a banner', (t) async {
    final backend = FakeBackend()
      ..firstName = null
      ..secondName = null
      ..declared = null
      ..identityStatus = 'none'
      ..declareRefusal = 'identity-declared';
    final h = await pumpApp(t, backend);
    h.container.read(routerProvider).go('/account/details');
    await t.pumpAndSettle();
    await fillIdentity(t, first: 'Grace', second: 'Wanjiru', id: '13579246');
    await tapVisible(t, find.widgetWithText(FilledButton, 'Save my details'));
    await t.pumpAndSettle();
    expect(find.text('Your names and ID number are already on your account'), findsOneWidget);
    expect(find.textContaining('contact support if they are wrong'), findsOneWidget);
  });

  testWidgets('the account screen shows the declared ID number masked and the verified standing', (t) async {
    final h = await pumpApp(t, FakeBackend()..identityStatus = 'verified');
    h.container.read(routerProvider).go('/account/details');
    await t.pumpAndSettle();
    expect(find.text('Jane'), findsOneWidget);
    expect(find.text('Wanjiru'), findsOneWidget);
    expect(find.text('•••••678'), findsOneWidget);
    expect(find.text('12345678'), findsNothing, reason: 'never the whole number');
    expect(find.text('Verified'), findsOneWidget);
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
    final dob = t.getTopLeft(find.widgetWithText(TextFormField, 'Date of birth')).dy;
    expect(
      first < second && second < id && id < dob,
      isTrue,
      reason: 'in that order, the date of birth last (ADR 0029)',
    );
    await t.tap(find.text('Sign in').first);
    await t.pumpAndSettle();
    expect(find.widgetWithText(TextFormField, 'First name'), findsNothing, reason: 'signing in asks for none of it');
    expect(find.widgetWithText(TextFormField, 'Date of birth'), findsNothing);
  });

  // ADR 0029: a date of birth under 18 is refused before anything is sent (the server refuses it too)
  testWidgets('sign-up refuses a date of birth under 18 on the field', (t) async {
    final backend = FakeBackend();
    await pumpApp(t, backend, signedIn: false);
    await t.tap(find.text('Create account').first);
    await t.pumpAndSettle();
    final now = DateTime.now();
    final minor = '${now.day.toString().padLeft(2, '0')}/${now.month.toString().padLeft(2, '0')}/${now.year - 17}';
    await fillIdentity(t, dateOfBirth: minor);
    await t.enterText(find.widgetWithText(TextFormField, 'M-Pesa phone number'), '0712345678');
    await t.enterText(find.widgetWithText(TextFormField, 'Email address'), 'jane@example.com');
    await t.enterText(find.widgetWithText(TextFormField, 'Password'), 'correct horse battery');
    await tapVisible(t, find.text('I am 18 or older'));
    await tapVisible(t, find.widgetWithText(FilledButton, 'Create account'));
    await t.pumpAndSettle();
    expect(find.text('You must be 18 or older to play.'), findsOneWidget);
    expect(backend.requests.where((r) => r.url.path == '/v1/auth/signup'), isEmpty);
  });
}
