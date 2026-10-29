import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:lottery_mobile/app/router.dart';
import 'package:lottery_mobile/core/auth/session_manager.dart';

import '../support/app_harness.dart';
import '../support/fake_backend.dart';
import '../support/fonts.dart';

/// Email addresses and self-service password resets (ADR 0021).
void main() {
  setUpAll(loadAppFonts);

  group('redirects', () {
    test('a signed-out player may ask for and use a reset link', () {
      expect(sessionRedirect(SessionStatus.signedOut, Uri.parse('/forgot-password')), isNull);
      expect(sessionRedirect(SessionStatus.signedOut, Uri.parse('/reset-password?token=abc')), isNull);
      expect(sessionRedirect(SessionStatus.signedOut, Uri.parse('/account/email')), startsWith('/sign-in'));
    });
    test('a signed-in player opening a reset link lands on Home', () {
      expect(sessionRedirect(SessionStatus.signedIn, Uri.parse('/reset-password?token=abc')), '/home');
    });
  });

  testWidgets('creating an account asks for an email address and sends it', (t) async {
    final backend = FakeBackend();
    await pumpApp(t, backend, signedIn: false);
    await t.tap(find.text('Create account').first);
    await t.pumpAndSettle();
    await t.enterText(find.widgetWithText(TextFormField, 'M-Pesa phone number'), '0712345678');
    await t.enterText(find.widgetWithText(TextFormField, 'Password'), 'correct horse battery');
    await t.enterText(find.widgetWithText(TextFormField, 'First name'), 'Jane');
    await t.enterText(find.widgetWithText(TextFormField, 'Second name'), 'Wanjiru');
    await t.tap(find.text('I am 18 or older'));
    await t.tap(find.widgetWithText(FilledButton, 'Create account'));
    await t.pumpAndSettle();
    expect(find.text('Enter an email address such as name@example.com.'), findsOneWidget);
    await t.enterText(find.widgetWithText(TextFormField, 'Email address'), 'Jane@Example.com');
    await t.tap(find.widgetWithText(FilledButton, 'Create account'));
    await t.pumpAndSettle();
    expect(find.text('Confirmed balance'), findsOneWidget);
    final signup = backend.requests.lastWhere((r) => r.url.path == '/v1/auth/signup');
    expect(signup.body, contains('"email":"Jane@Example.com"'));
  });

  testWidgets('an address that already has an account is refused on the field', (t) async {
    await pumpApp(t, FakeBackend(), signedIn: false);
    await t.tap(find.text('Create account').first);
    await t.pumpAndSettle();
    await t.enterText(find.widgetWithText(TextFormField, 'M-Pesa phone number'), '0712345678');
    await t.enterText(find.widgetWithText(TextFormField, 'Email address'), 'taken@example.com');
    await t.enterText(find.widgetWithText(TextFormField, 'Password'), 'correct horse battery');
    await t.enterText(find.widgetWithText(TextFormField, 'First name'), 'Jane');
    await t.enterText(find.widgetWithText(TextFormField, 'Second name'), 'Wanjiru');
    await t.tap(find.text('I am 18 or older'));
    await t.tap(find.widgetWithText(FilledButton, 'Create account'));
    await t.pumpAndSettle();
    expect(find.textContaining('This email address already has an account'), findsOneWidget);
  });

  testWidgets('a forgotten password: the address is taken, the answer is the same for any address', (t) async {
    final backend = FakeBackend();
    await pumpApp(t, backend, signedIn: false);
    await t.tap(find.text('Forgotten your password?'));
    await t.pumpAndSettle();
    expect(find.text('Reset your password'), findsOneWidget);
    await t.enterText(find.widgetWithText(TextFormField, 'Email address'), 'jane@example.com');
    await t.tap(find.widgetWithText(FilledButton, 'Email me a link'));
    await t.pumpAndSettle();
    expect(find.text('If that address has an account, a link is on its way.'), findsOneWidget);
    expect(backend.resetRequests, ['jane@example.com']);
    await t.tap(find.widgetWithText(FilledButton, 'Back to sign in'));
    await t.pumpAndSettle();
    expect(find.text('Good to see you'), findsOneWidget);
  });

  testWidgets('without email configured on the server, the player is told to ask for a temporary password', (t) async {
    await pumpApp(t, FakeBackend()..resetsConfigured = false, signedIn: false);
    await t.tap(find.text('Forgotten your password?'));
    await t.pumpAndSettle();
    await t.enterText(find.widgetWithText(TextFormField, 'Email address'), 'jane@example.com');
    await t.tap(find.widgetWithText(FilledButton, 'Email me a link'));
    await t.pumpAndSettle();
    expect(find.text('Password reset by email is not configured'), findsOneWidget);
  });

  testWidgets('the emailed link opens a page for a new password, then sign-in says so', (t) async {
    final backend = FakeBackend();
    final h = await pumpApp(t, backend, signedIn: false);
    h.container.read(routerProvider).go('/reset-password?token=good-link');
    await t.pumpAndSettle();
    expect(find.text('Choose a new password'), findsOneWidget);
    await t.enterText(find.widgetWithText(TextFormField, 'New password'), 'short');
    await t.tap(find.widgetWithText(FilledButton, 'Save new password'));
    await t.pumpAndSettle();
    expect(find.text('Use at least 8 characters.'), findsOneWidget);
    await t.enterText(find.widgetWithText(TextFormField, 'New password'), 'a brand new password');
    await t.tap(find.widgetWithText(FilledButton, 'Save new password'));
    await t.pumpAndSettle();
    expect(find.text('Your password is changed. Sign in with it.'), findsOneWidget);
    final reset = backend.requests.lastWhere((r) => r.url.path == '/v1/auth/password/reset');
    expect(reset.body, contains('"token":"good-link"'));
  });

  testWidgets('a used or expired link says so and offers a new one', (t) async {
    final h = await pumpApp(t, FakeBackend(), signedIn: false);
    h.container.read(routerProvider).go('/reset-password?token=stale');
    await t.pumpAndSettle();
    await t.enterText(find.widgetWithText(TextFormField, 'New password'), 'a brand new password');
    await t.tap(find.widgetWithText(FilledButton, 'Save new password'));
    await t.pumpAndSettle();
    expect(find.text('This link no longer works'), findsOneWidget);
    await t.tap(find.widgetWithText(FilledButton, 'Ask for a new link'));
    await t.pumpAndSettle();
    expect(find.text('Reset your password'), findsOneWidget);
  });

  testWidgets('the account shows the address, and changing it takes the password', (t) async {
    final backend = FakeBackend();
    await pumpApp(t, backend);
    await t.tap(find.text('Account').last);
    await t.pumpAndSettle();
    expect(find.text('jane@example.com'), findsOneWidget);
    await t.tap(find.text('Email address'));
    await t.pumpAndSettle();
    await t.enterText(find.widgetWithText(TextFormField, 'Email address'), 'jane2@example.com');
    await t.enterText(find.widgetWithText(TextFormField, 'Your password'), 'wrong');
    await t.tap(find.widgetWithText(FilledButton, 'Save'));
    await t.pumpAndSettle();
    expect(find.text('Current password is incorrect'), findsOneWidget);
    await t.enterText(find.widgetWithText(TextFormField, 'Your password'), 'correct horse battery');
    await t.tap(find.widgetWithText(FilledButton, 'Save'));
    await t.pumpAndSettle();
    expect(find.text('Email address saved.'), findsOneWidget);
    expect(backend.email, 'jane2@example.com');
  });

  testWidgets('an account from before, or one that signs in with Google, adds an address without a password', (
    t,
  ) async {
    final backend = FakeBackend()
      ..email = null
      ..hasPassword = false;
    await pumpApp(t, backend);
    await t.tap(find.text('Account').last);
    await t.pumpAndSettle();
    expect(find.text('Add one so you can reset a forgotten password'), findsOneWidget);
    await t.tap(find.text('Email address'));
    await t.pumpAndSettle();
    expect(find.text('Add an email address'), findsOneWidget);
    expect(find.widgetWithText(TextFormField, 'Your password'), findsNothing);
    await t.enterText(find.widgetWithText(TextFormField, 'Email address'), 'g@example.com');
    await t.tap(find.widgetWithText(FilledButton, 'Add email address'));
    await t.pumpAndSettle();
    expect(backend.email, 'g@example.com');
  });
}
