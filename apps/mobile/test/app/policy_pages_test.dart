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
      expect(backend.opened.last, Uri.parse('https://tajirika.co.ke/$slug/'), reason: title);
    }
  });

  testWidgets('closing an account says what the law makes us keep, and links to the details', (t) async {
    final backend = FakeBackend();
    final h = await pumpApp(t, backend);
    h.container.read(routerProvider).go('/account/close');
    await t.pumpAndSettle();
    expect(find.textContaining('the law makes gambling operators keep records'), findsOneWidget);
    await tapVisible(t, find.widgetWithText(TextButton, 'What is kept, and why'));
    expect(backend.opened.single, Uri.parse('https://tajirika.co.ke/delete-account/'));
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
