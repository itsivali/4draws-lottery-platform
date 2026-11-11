import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart' hide testWidgets;
import 'package:flutter_test/flutter_test.dart' as flutter_test show testWidgets;

import '../support/app_harness.dart';
import '../support/fake_backend.dart';
import '../support/fonts.dart';

// Golden images of whole screens, for review of the design (MASTER.md) and to catch visual regressions.
void main() {
  setUpAll(loadAppFonts);

  // the soft shadows and glows are part of the design, so these images paint them (tests flatten shadows by default)
  void testWidgets(String description, Future<void> Function(WidgetTester) body) => flutter_test.testWidgets(
    description,
    (t) async {
      debugDisableShadows = false;
      try {
        await body(t);
      } finally {
        debugDisableShadows = true;
      }
    },
  );

  Future<void> shot(WidgetTester t, String name) async {
    await t.pumpAndSettle();
    await expectLater(find.byType(MaterialApp), matchesGoldenFile('goldens/$name.png'));
  }

  testWidgets('sign in', (t) async {
    await pumpApp(t, FakeBackend(), signedIn: false);
    await shot(t, 'sign_in_phone');
  });

  testWidgets('sign in on a desktop browser', (t) async {
    await pumpApp(t, FakeBackend(), signedIn: false, size: const Size(1440, 900));
    await shot(t, 'sign_in_desktop');
  });

  testWidgets('sign in with Google: the phone number step', (t) async {
    await pumpApp(t, FakeBackend(), signedIn: false);
    await t.tap(find.widgetWithText(OutlinedButton, 'Continue with Google'));
    await shot(t, 'sign_in_phone_google_number');
  });

  testWidgets('home', (t) async {
    final b = FakeBackend(balance: 1250);
    await pumpApp(t, b);
    await shot(t, 'home_phone');
  });

  testWidgets('home on a small phone', (t) async {
    await pumpApp(t, FakeBackend(balance: 1250), size: const Size(360, 740));
    await shot(t, 'home_small_phone');
  });

  testWidgets('account', (t) async {
    await pumpApp(t, FakeBackend());
    await t.tap(find.text('Account').last);
    await shot(t, 'account_phone');
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

  testWidgets('play blocked (number not confirmed)', (t) async {
    await pumpApp(t, FakeBackend(phoneVerified: false));
    await t.tap(find.text('Play').last);
    await shot(t, 'play_phone_blocked');
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
