import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:lottery_mobile/app/router.dart';

import '../support/app_harness.dart';
import '../support/fake_backend.dart';
import '../support/fonts.dart';
import '../support/journeys.dart';

/// The account as a profile (owner, 2026-10-31): the devices signed in, and the ways into the account.
void main() {
  setUpAll(loadAppFonts);

  testWidgets('the account screen is a profile: initials, name, number, email, and the sections', (t) async {
    await pumpApp(t, FakeBackend());
    await t.tap(find.text('Account').last);
    await t.pumpAndSettle();
    expect(find.text('JW'), findsOneWidget, reason: 'the initials avatar');
    expect(find.text('Jane Wanjiru'), findsOneWidget);
    expect(find.text('jane@example.com'), findsOneWidget, reason: 'in the header, once');
    for (final section in ['YOUR DETAILS', 'SIGN-IN AND SECURITY', 'PLAY SAFELY', 'PREFERENCES', 'ACCOUNT']) {
      expect(find.text(section), findsOneWidget, reason: section);
    }
    for (final row in ['Your details', 'Identity verification', 'Devices signed in', 'Sign-in methods']) {
      expect(find.text(row), findsOneWidget, reason: row);
    }
  });

  testWidgets('the devices signed in are listed, this one marked, and the others can be signed out', (t) async {
    final backend = FakeBackend();
    final h = await pumpApp(t, backend);
    h.container.read(routerProvider).go('/account/devices');
    await t.pumpAndSettle();
    expect(find.text('Android phone'), findsOneWidget);
    expect(find.text('Web browser'), findsOneWidget);
    expect(find.text('This device'), findsOneWidget);
    expect(find.textContaining('Signed in 20 Oct 2026, 09:00 EAT'), findsOneWidget);
    await tapVisible(t, find.widgetWithText(OutlinedButton, 'Sign out other devices'));
    await t.pumpAndSettle();
    expect(find.text('Sign out other devices?'), findsOneWidget);
    expect(find.text('1 other device'), findsOneWidget);
    await t.tap(find.widgetWithText(FilledButton, 'Sign them out'));
    await t.pumpAndSettle();
    expect(
      backend.requests.where((r) => r.method == 'POST' && r.url.path == '/v1/account/sessions/revoke-others').length,
      1,
    );
    expect(find.text('Web browser'), findsNothing, reason: 'the other device is gone');
    expect(find.text('Android phone'), findsOneWidget, reason: 'this one stays');
    expect(find.text('Only this device is signed in.'), findsOneWidget);
    expect(t.widget<OutlinedButton>(find.widgetWithText(OutlinedButton, 'Sign out other devices')).enabled, isFalse);
  });

  testWidgets('the sign-in methods show the password and the linked Google or Apple identity with its date', (t) async {
    final backend = FakeBackend()..linkedSignIns.add({'provider': 'google', 'linked_at': '2026-10-20T10:00:00Z'});
    final h = await pumpApp(t, backend);
    h.container.read(routerProvider).go('/account/sign-ins');
    await t.pumpAndSettle();
    expect(find.text('Password'), findsOneWidget);
    expect(find.text('Set'), findsOneWidget);
    expect(find.text('Linked 20 Oct 2026, 13:00 EAT'), findsOneWidget);
    expect(find.text('Not linked'), findsOneWidget, reason: 'Apple');
    expect(find.textContaining('linked from the sign-in screen'), findsOneWidget);
  });

  testWidgets('an account without a password says so on its sign-in methods', (t) async {
    final backend = FakeBackend()
      ..hasPassword = false
      ..linkedSignIns.add({'provider': 'apple', 'linked_at': '2026-10-20T10:00:00Z'});
    final h = await pumpApp(t, backend);
    h.container.read(routerProvider).go('/account/sign-ins');
    await t.pumpAndSettle();
    expect(find.text('Not set: you sign in with Google or Apple. Tap to set one'), findsOneWidget);
    expect(find.text('Not linked'), findsOneWidget, reason: 'Google');
  });
}
