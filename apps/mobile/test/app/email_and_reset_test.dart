import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:lottery_mobile/app/router.dart';
import 'package:lottery_mobile/core/auth/session_manager.dart';

import '../support/app_harness.dart';
import '../support/fake_backend.dart';
import '../support/fonts.dart';
import '../support/journeys.dart';

/// Email addresses and self-service password resets (ADR 0021).
void main() {
  setUpAll(loadAppFonts);

  group('redirects', () {
    test('a signed-out player may ask for and use a reset link', () {
      expect(sessionRedirect(SessionStatus.signedOut, Uri.parse('/forgot-password')), isNull);
      expect(sessionRedirect(SessionStatus.signedOut, Uri.parse('/reset-password?token=abc')), isNull);
      expect(sessionRedirect(SessionStatus.signedOut, Uri.parse('/account/email')), startsWith('/sign-in'));
    });
    test('a signed-in player may open a reset link too: they can ask for one from Account', () {
      expect(sessionRedirect(SessionStatus.signedIn, Uri.parse('/reset-password?token=abc')), isNull);
    });
    test('the link that confirms an address opens signed in or out (ADR 0040)', () {
      expect(sessionRedirect(SessionStatus.signedOut, Uri.parse('/verify-email?token=abc')), isNull);
      expect(sessionRedirect(SessionStatus.signedIn, Uri.parse('/verify-email?token=abc')), isNull);
    });
    test('an emailed link survives the moment the app spends starting', () {
      for (final link in ['/verify-email?token=abc', '/reset-password?token=abc']) {
        final starting = sessionRedirect(SessionStatus.restoring, Uri.parse(link))!;
        expect(starting, startsWith('/starting?from='));
        expect(sessionRedirect(SessionStatus.signedOut, Uri.parse(starting)), link, reason: link);
      }
      final starting = sessionRedirect(SessionStatus.restoring, Uri.parse('/verify-email?token=abc'))!;
      expect(sessionRedirect(SessionStatus.signedIn, Uri.parse(starting)), '/verify-email?token=abc');
      final reset = sessionRedirect(SessionStatus.restoring, Uri.parse('/reset-password?token=abc'))!;
      expect(sessionRedirect(SessionStatus.signedIn, Uri.parse(reset)), '/reset-password?token=abc');
      // and nothing else rides through "from"
      expect(
        sessionRedirect(SessionStatus.signedOut, Uri.parse('/starting?from=https%3A%2F%2Fevil.example%2Fverify-email')),
        '/sign-in',
      );
    });
  });

  testWidgets('creating an account asks for an email address and sends it', (t) async {
    final backend = FakeBackend();
    await pumpApp(t, backend, signedIn: false);
    await t.tap(find.text('Create account').first);
    await t.pumpAndSettle();
    await fillIdentity(t);
    await t.enterText(find.widgetWithText(TextFormField, 'M-Pesa phone number'), '0712345678');
    await t.enterText(find.widgetWithText(TextFormField, 'Password'), 'correct horse battery');
    await tapVisible(t, find.text('I am 18 or older'));
    await tapVisible(t, find.widgetWithText(FilledButton, 'Create account'));
    await t.pumpAndSettle();
    expect(find.text('Enter an email address such as name@example.com.'), findsOneWidget);
    await t.enterText(find.widgetWithText(TextFormField, 'Email address'), 'Jane@Example.com');
    await tapVisible(t, find.widgetWithText(FilledButton, 'Create account'));
    await t.pumpAndSettle();
    expect(find.text('Confirmed balance'), findsOneWidget);
    final signup = backend.requests.lastWhere((r) => r.url.path == '/v1/auth/signup');
    expect(signup.body, contains('"email":"Jane@Example.com"'));
  });

  testWidgets('an address that already has an account is refused on the field', (t) async {
    await pumpApp(t, FakeBackend(), signedIn: false);
    await t.tap(find.text('Create account').first);
    await t.pumpAndSettle();
    await fillIdentity(t);
    await t.enterText(find.widgetWithText(TextFormField, 'M-Pesa phone number'), '0712345678');
    await t.enterText(find.widgetWithText(TextFormField, 'Email address'), 'taken@example.com');
    await t.enterText(find.widgetWithText(TextFormField, 'Password'), 'correct horse battery');
    await tapVisible(t, find.text('I am 18 or older'));
    await tapVisible(t, find.widgetWithText(FilledButton, 'Create account'));
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
    expect(find.text('Email address saved. We emailed you a link to confirm it.'), findsOneWidget);
    expect(backend.email, 'jane2@example.com');
    // a changed address is a new claim: the account says it is not confirmed yet (ADR 0040)
    expect(find.text('Email not confirmed yet'), findsOneWidget);
    expect(find.text('Not confirmed yet: open the link we emailed you'), findsOneWidget);
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

  testWidgets('the emailed link confirms the address, signed out, and says so', (t) async {
    final backend = FakeBackend()..emailVerified = false;
    final h = await pumpApp(t, backend, signedIn: false);
    h.container.read(routerProvider).go('/verify-email?token=good-link');
    await t.pumpAndSettle();
    expect(find.text('Your email address is confirmed'), findsOneWidget);
    expect(backend.emailVerified, isTrue);
    final sent = backend.requests.where((r) => r.url.path == '/v1/auth/email/verify');
    expect(sent, hasLength(1), reason: 'the link is used once');
    expect(sent.single.body, contains('"token":"good-link"'));
    await t.tap(find.widgetWithText(FilledButton, 'Open 4draws'));
    await t.pumpAndSettle();
    expect(find.text('Sign in'), findsWidgets);
  });

  testWidgets('a used or expired confirmation link says what to do', (t) async {
    final backend = FakeBackend()..emailVerified = false;
    final h = await pumpApp(t, backend, signedIn: false);
    h.container.read(routerProvider).go('/verify-email?token=stale');
    await t.pumpAndSettle();
    expect(find.text('This link no longer works'), findsOneWidget);
    expect(backend.emailVerified, isFalse);
    h.container.read(routerProvider).go('/verify-email');
    await t.pumpAndSettle();
    expect(find.text('This link no longer works'), findsOneWidget);
  });

  testWidgets('signed in, the link confirms the address and the account shows it', (t) async {
    final backend = FakeBackend()..emailVerified = false;
    final h = await pumpApp(t, backend);
    await t.tap(find.text('Account').last);
    await t.pumpAndSettle();
    expect(find.text('Email not confirmed yet'), findsOneWidget);
    h.container.read(routerProvider).go('/verify-email?token=good-link');
    await t.pumpAndSettle();
    expect(find.text('Your email address is confirmed'), findsOneWidget);
    await t.tap(find.widgetWithText(FilledButton, 'Open 4draws'));
    await t.pumpAndSettle();
    await t.tap(find.text('Account').last);
    await t.pumpAndSettle();
    expect(find.text('Email not confirmed yet'), findsNothing);
    expect(find.text('Where a reset link goes'), findsOneWidget);
  });

  testWidgets('an unconfirmed address can be sent its link again, and a copy of the data waits for it', (t) async {
    final backend = FakeBackend()..emailVerified = false;
    final h = await pumpApp(t, backend);
    h.container.read(routerProvider).go('/account/email');
    await t.pumpAndSettle();
    expect(find.text('Confirm this address'), findsOneWidget);
    await t.tap(find.widgetWithText(FilledButton, 'Send the link again'));
    await t.pumpAndSettle();
    expect(backend.verificationLinks, 1);
    expect(find.text('A new link is on its way'), findsOneWidget);

    h.container.read(routerProvider).go('/account/data');
    await t.pumpAndSettle();
    expect(find.text('Confirm your email address first'), findsOneWidget);
    expect(find.widgetWithText(FilledButton, 'Email me my data'), findsNothing);
  });

  testWidgets('a confirmed address says so', (t) async {
    final h = await pumpApp(t, FakeBackend());
    h.container.read(routerProvider).go('/account/email');
    await t.pumpAndSettle();
    expect(find.text('This address is confirmed'), findsOneWidget);
    expect(find.text('Send the link again'), findsNothing);
  });

  testWidgets('a signed-in player who forgot their password is emailed the reset link, and the link works', (t) async {
    final backend = FakeBackend();
    final h = await pumpApp(t, backend);
    h.container.read(routerProvider).go('/account/password');
    await t.pumpAndSettle();
    expect(find.text('Forgotten your current password?'), findsOneWidget);
    await tapVisible(t, find.widgetWithText(FilledButton, 'Email me a reset link'));
    await t.pumpAndSettle();
    expect(backend.resetRequests, ['jane@example.com']);
    expect(find.text('A link is on its way to jane@example.com'), findsOneWidget);

    // the link opens while signed in; using it ends this session and lands on sign-in
    h.container.read(routerProvider).go('/reset-password?token=good-link');
    await t.pumpAndSettle();
    expect(find.text('Choose a new password'), findsOneWidget);
    await t.enterText(find.widgetWithText(TextFormField, 'New password'), 'a brand new password');
    await t.tap(find.widgetWithText(FilledButton, 'Save new password'));
    await t.pumpAndSettle();
    expect(find.text('Your password is changed. Sign in with it.'), findsOneWidget);
  });

  testWidgets('without an email address the screen offers to add one', (t) async {
    final h = await pumpApp(t, FakeBackend()..email = null);
    h.container.read(routerProvider).go('/account/password');
    await t.pumpAndSettle();
    expect(find.text('Email me a reset link'), findsNothing);
    expect(find.widgetWithText(FilledButton, 'Add an email address'), findsOneWidget);
  });
}
