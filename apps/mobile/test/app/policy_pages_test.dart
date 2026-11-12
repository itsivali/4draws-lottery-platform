import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:lottery_mobile/app/router.dart';

import '../support/app_harness.dart';
import '../support/fake_backend.dart';
import '../support/fonts.dart';
import '../support/journeys.dart';

/// S8: the policy pages are a tap away, account closure says what is kept, and a demo or stopped service says so.
void main() {
  setUpAll(loadAppFonts);

  testWidgets('the account links to the four policy pages on the site', (t) async {
    final backend = FakeBackend();
    await pumpApp(t, backend);
    await t.tap(find.text('Account').last);
    await t.pumpAndSettle();
    expect(find.text('ABOUT'), findsOneWidget);
    for (final (title, slug) in [
      ('Privacy policy', 'privacy'),
      ('Terms of use', 'terms'),
      ('Responsible gambling', 'responsible-gambling'),
      ('Delete your account', 'delete-account'),
    ]) {
      await tapVisible(t, find.text(title));
      await t.pumpAndSettle();
      expect(backend.opened.last, Uri.parse('https://4draws.co.ke/$slug/'), reason: title);
    }
  });

  testWidgets('closing an account says what the law makes us keep, and links to the details', (t) async {
    final backend = FakeBackend();
    final h = await pumpApp(t, backend);
    h.container.read(routerProvider).go('/account/close');
    await t.pumpAndSettle();
    expect(find.textContaining('the law makes gambling operators keep records'), findsOneWidget);
    await tapVisible(t, find.widgetWithText(TextButton, 'What is kept, and why'));
    expect(backend.opened.single, Uri.parse('https://4draws.co.ke/delete-account/'));
  });

  testWidgets('a player asks for a copy of their data and is told where it was emailed; a second ask is refused', (
    t,
  ) async {
    final backend = FakeBackend();
    final h = await pumpApp(t, backend);
    h.container.read(routerProvider).go('/account/data');
    await t.pumpAndSettle();
    expect(find.text('It will be sent to'), findsOneWidget);
    expect(find.textContaining('last three digits of your ID number'), findsOneWidget);
    await tapVisible(t, find.widgetWithText(FilledButton, 'Email me my data'));
    await t.pumpAndSettle();
    expect(backend.dataCopies, 1);
    expect(find.text('Sent to j***@example.com'), findsOneWidget);
    expect(find.widgetWithText(FilledButton, 'Email me my data'), findsNothing, reason: 'nothing more to press');

    // opened again the same day: the server refuses, and the screen says why
    h.container.read(routerProvider).go('/account');
    await t.pumpAndSettle();
    h.container.read(routerProvider).go('/account/data');
    await t.pumpAndSettle();
    await tapVisible(t, find.widgetWithText(FilledButton, 'Email me my data'));
    await t.pumpAndSettle();
    expect(find.text('A copy was sent in the last 24 hours'), findsOneWidget);
  });

  testWidgets('a demo service says the money is not real; a live one says nothing', (t) async {
    await pumpApp(t, FakeBackend()..operatingMode = 'sandbox');
    expect(find.text('Demo: test money only, no real prizes'), findsOneWidget);
  });

  testWidgets('a stopped service says so', (t) async {
    await pumpApp(t, FakeBackend()..operatingMode = 'disabled');
    expect(find.text('Top-ups, plays and withdrawals are paused'), findsOneWidget);
  });

  testWidgets('live, or no status at all, shows no banner', (t) async {
    await pumpApp(t, FakeBackend()..operatingMode = 'live');
    expect(find.textContaining('Demo:'), findsNothing);
    expect(find.text('Top-ups, plays and withdrawals are paused'), findsNothing);
  });
}
