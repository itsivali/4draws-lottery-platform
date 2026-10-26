import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';

import '../support/app_harness.dart';
import '../support/fake_backend.dart';
import '../support/fonts.dart';

// Golden images of whole screens, for review of the design (MASTER.md) and to catch visual regressions.
void main() {
  setUpAll(loadAppFonts);

  Future<void> shot(WidgetTester t, String name) async {
    await t.pumpAndSettle();
    await expectLater(find.byType(MaterialApp), matchesGoldenFile('goldens/$name.png'));
  }

  testWidgets('sign in', (t) async {
    await pumpApp(t, FakeBackend(), signedIn: false);
    await shot(t, 'sign_in_phone');
  });

  testWidgets('home', (t) async {
    final b = FakeBackend(balance: 1250);
    await pumpApp(t, b);
    await shot(t, 'home_phone');
  });

  testWidgets('play, ready', (t) async {
    await pumpApp(t, FakeBackend());
    await t.tap(find.text('Play').last);
    await shot(t, 'play_phone_ready');
  });

  testWidgets('play, lucky result revealed', (t) async {
    final b = FakeBackend()..spins.add(0);
    await pumpApp(t, b);
    await t.tap(find.text('Play').last);
    await t.pumpAndSettle();
    await t.tap(find.widgetWithText(FilledButton, 'Play for KES 100').first);
    await t.pumpAndSettle();
    await t.tap(find.widgetWithText(FilledButton, 'Play for KES 100').last);
    await pumpUntilFound(t, find.text("Lucky slot — today's chances are doubled"));
    await shot(t, 'play_phone_lucky');
  });

  testWidgets('play confirmation', (t) async {
    await pumpApp(t, FakeBackend());
    await t.tap(find.text('Play').last);
    await t.pumpAndSettle();
    await t.tap(find.widgetWithText(FilledButton, 'Play for KES 100').first);
    await shot(t, 'play_phone_confirm');
  });

  testWidgets('play blocked (ID not verified), dark', (t) async {
    t.platformDispatcher.platformBrightnessTestValue = Brightness.dark;
    await pumpApp(t, FakeBackend(kyc: 'none'));
    await t.tap(find.text('Play').last);
    await shot(t, 'play_phone_blocked_dark');
  });

  testWidgets('wallet', (t) async {
    await pumpApp(
      t,
      FakeBackend(balance: 850)
        ..txs.addAll([
          {
            'id': 'tx-1',
            'kind': 'deposit',
            'amount_kes': 1000,
            'reference_type': 'payment',
            'reference_id': 'p',
            'created_at': '2026-10-26T09:00:00+03:00',
          },
          {
            'id': 'tx-2',
            'kind': 'deposit_excise',
            'amount_kes': -50,
            'reference_type': 'payment',
            'reference_id': 'p',
            'created_at': '2026-10-26T09:00:00+03:00',
          },
          {
            'id': 'tx-3',
            'kind': 'stake',
            'amount_kes': -100,
            'reference_type': 'ticket',
            'reference_id': 't',
            'created_at': '2026-10-26T10:00:00+03:00',
          },
        ]),
    );
    await t.tap(find.text('Wallet').last);
    await shot(t, 'wallet_phone');
  });

  testWidgets('draws', (t) async {
    await pumpApp(t, FakeBackend());
    await t.tap(find.text('Draws').last);
    await shot(t, 'draws_phone');
  });

  testWidgets('play on a desktop browser', (t) async {
    await pumpApp(t, FakeBackend(), size: const Size(1440, 900));
    await t.tap(find.text('Play').last);
    await shot(t, 'play_desktop');
  });
}
