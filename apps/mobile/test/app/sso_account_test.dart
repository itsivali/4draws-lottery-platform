import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:lottery_mobile/app/router.dart';

import '../support/app_harness.dart';
import '../support/fake_backend.dart';
import '../support/fonts.dart';
import '../support/journeys.dart';

/// ADR 0035: an account that signs in with Google shows the Google photo, and can set a password right after signing in.
void main() {
  setUpAll(loadAppFonts);

  testWidgets('the account shows the Google photo over the initials', (t) async {
    await pumpApp(t, FakeBackend()..photoUrl = 'https://lh3.googleusercontent.com/a/jane=s96-c');
    await t.tap(find.text('Account').last);
    await t.pumpAndSettle();
    expect(find.byKey(const Key('account-photo')), findsOneWidget);
    expect(find.text('JW'), findsOneWidget, reason: 'the initials stay underneath, for when the photo cannot load');
  });

  testWidgets('a photo from anywhere but Google is not shown', (t) async {
    await pumpApp(t, FakeBackend()..photoUrl = 'https://evil.example/pixel.png');
    await t.tap(find.text('Account').last);
    await t.pumpAndSettle();
    expect(find.byKey(const Key('account-photo')), findsNothing);
  });

  testWidgets('a Google account sets a password from its sign-in methods', (t) async {
    final backend = FakeBackend()..hasPassword = false;
    final h = await pumpApp(t, backend);
    h.container.read(routerProvider).go('/account/sign-ins');
    await t.pumpAndSettle();
    expect(find.textContaining('Tap to set one'), findsOneWidget);
    await tapVisible(t, find.text('Password'));
    await t.pumpAndSettle();
    expect(find.text('Set a password'), findsWidgets);
    expect(find.text('Current password'), findsNothing, reason: 'there is no current one');
    await t.enterText(find.widgetWithText(TextFormField, 'New password'), 'a long new passphrase');
    await tapVisible(t, find.widgetWithText(FilledButton, 'Set password'));
    await t.pumpAndSettle();
    expect(backend.requests.where((r) => r.method == 'POST' && r.url.path == '/v1/auth/password/set').length, 1);
    expect(find.textContaining('Password set.'), findsOneWidget);
    expect(backend.hasPassword, isTrue);
  });

  testWidgets('long after signing in, setting a password says to sign in again', (t) async {
    final backend = FakeBackend()
      ..hasPassword = false
      ..signInStale = true;
    final h = await pumpApp(t, backend);
    h.container.read(routerProvider).go('/account/password?set=1');
    await t.pumpAndSettle();
    await t.enterText(find.widgetWithText(TextFormField, 'New password'), 'a long new passphrase');
    await tapVisible(t, find.widgetWithText(FilledButton, 'Set password'));
    await t.pumpAndSettle();
    expect(find.textContaining('Sign in again to set a password'), findsOneWidget);
    expect(backend.hasPassword, isFalse);
  });
}
