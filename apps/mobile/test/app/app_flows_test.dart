import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:lottery_mobile/app/router.dart';
import 'package:lottery_mobile/core/platform/haptics.dart';
import 'package:lottery_mobile/features/wallet/application/wallet_providers.dart';

import '../support/app_harness.dart';
import '../support/fake_backend.dart';
import '../support/fonts.dart';

Future<void> goToPlay(WidgetTester t) async {
  await t.tap(find.text('Play').last);
  await t.pumpAndSettle();
}

Future<void> playOnce(WidgetTester t) async {
  await t.tap(find.widgetWithText(FilledButton, 'Play for KES 100').first);
  await t.pumpAndSettle();
  expect(find.text('Play for KES 100?'), findsOneWidget, reason: 'the price is confirmed before any paid play');
  await t.tap(find.widgetWithText(FilledButton, 'Play for KES 100').last);
  await t.pump();
}

void main() {
  setUpAll(loadAppFonts);

  testWidgets('signing in lands on Home with the confirmed balance', (t) async {
    final backend = FakeBackend(balance: 1250);
    await pumpApp(t, backend, signedIn: false);
    expect(find.text('Welcome back'), findsOneWidget);
    await t.enterText(find.widgetWithText(TextFormField, 'M-Pesa phone number'), '0712345678');
    await t.enterText(find.widgetWithText(TextFormField, 'Password'), 'correct horse battery');
    await t.tap(find.widgetWithText(FilledButton, 'Sign in'));
    await t.pumpAndSettle();
    expect(find.text('KES 1,250'), findsOneWidget);
    expect(find.text('Confirmed balance'), findsOneWidget);
  });

  testWidgets('a wrong password shows the server message and stays on sign-in', (t) async {
    await pumpApp(t, FakeBackend(), signedIn: false);
    await t.enterText(find.widgetWithText(TextFormField, 'M-Pesa phone number'), '0712345678');
    await t.enterText(find.widgetWithText(TextFormField, 'Password'), 'not the password');
    await t.tap(find.widgetWithText(FilledButton, 'Sign in'));
    await t.pumpAndSettle();
    expect(find.text('Invalid phone number or password'), findsOneWidget);
  });

  testWidgets('a play: price shown and confirmed, the wheel spins, the server result is revealed', (t) async {
    final backend = FakeBackend()..spins.add(7);
    final h = await pumpApp(t, backend);
    await goToPlay(t);
    expect(find.text('KES 100'), findsWidgets);
    expect(find.text('4 of 4'), findsOneWidget);
    await playOnce(t);
    expect(
      find.textContaining(RegExp('Recording your play|the wheel is showing your spin')),
      findsOneWidget,
      reason: 'while the wheel turns, no slot is spelled out',
    );
    expect(find.textContaining('slot 7'), findsNothing);
    await pumpUntilFound(t, find.text('Play recorded'));
    expect(find.textContaining('slot 7 of 12'), findsWidgets);
    expect(find.textContaining('1 draw chance today'), findsOneWidget);
    expect(find.textContaining('This is not a prize'), findsOneWidget);
    expect(find.text('3 of 4'), findsOneWidget);
    expect(find.textContaining('Balance after this play: KES 900'), findsOneWidget);
    expect(backend.plays, 1);
    expect(h.haptics.events.first, HapticEvent.start);
    expect(h.haptics.events.last, HapticEvent.stop);
    expect(h.haptics.events.where((e) => e == HapticEvent.stop), hasLength(1));
    await t.pumpAndSettle();
  });

  testWidgets("a lucky slot says today's chances are doubled — never that money was won", (t) async {
    final backend = FakeBackend()..spins.add(0);
    await pumpApp(t, backend);
    await goToPlay(t);
    await playOnce(t);
    await pumpUntilFound(t, find.text("Lucky slot — today's chances are doubled"));
    expect(find.textContaining('2 draw chances today'), findsOneWidget);
    expect(find.textContaining('won'), findsNothing);
    expect(find.textContaining('Win'), findsNothing);
    await t.pumpAndSettle();
  });

  testWidgets('a lost answer is reconciled with the same key: one play, one charge', (t) async {
    final backend = FakeBackend()
      ..spins.add(3)
      ..loseNextPlayAnswer = true;
    await pumpApp(t, backend);
    await goToPlay(t);
    await playOnce(t);
    await pumpUntilFound(t, find.text("We're checking whether your play went through"));
    expect(find.textContaining('slot 3'), findsNothing, reason: 'no outcome is shown while unsure');
    await pumpUntilFound(t, find.text('Play recorded'));
    expect(find.textContaining('confirmed this play after the connection dropped'), findsOneWidget);
    expect(backend.plays, 1);
    expect(backend.balance, 900);
    final keys = backend.requests
        .where((r) => r.url.path == '/v1/plays')
        .map((r) => r.headers['Idempotency-Key'])
        .toSet();
    expect(keys, hasLength(1));
    await t.pumpAndSettle();
  });

  testWidgets('an unconfirmed number turns the button off and says why; an unverified ID does not', (t) async {
    await pumpApp(t, FakeBackend(phoneVerified: false));
    await goToPlay(t);
    expect(find.text('Confirm your number with a first top-up'), findsOneWidget);
    final button = t.widget<FilledButton>(find.widgetWithText(FilledButton, 'Play for KES 100'));
    expect(button.onPressed, isNull);
  });

  testWidgets('an unverified ID does not stop a play; Home suggests verifying before a withdrawal', (t) async {
    await pumpApp(t, FakeBackend(kyc: 'none'));
    expect(find.text('Verify your identity before you withdraw'), findsOneWidget);
    await goToPlay(t);
    expect(find.text('Verify your identity to play'), findsNothing);
    final button = t.widget<FilledButton>(find.widgetWithText(FilledButton, 'Play for KES 100').first);
    expect(button.onPressed, isNotNull);
  });

  testWidgets('after four plays the day is over, and it says when it resets', (t) async {
    final backend = FakeBackend()..plays = 4;
    await pumpApp(t, backend);
    await goToPlay(t);
    expect(find.text("You've played 4 times today"), findsOneWidget);
    expect(find.text('Plays reset at midnight Nairobi time.'), findsOneWidget);
  });

  testWidgets('a top-up stays pending until the server confirms it, then the balance updates', (t) async {
    final backend = FakeBackend(balance: 0)..topUpPendingPolls = 2;
    final h = await pumpApp(t, backend);
    await t.tap(find.text('Wallet').last);
    await t.pumpAndSettle();
    await t.tap(find.widgetWithText(FilledButton, 'Top up'));
    await t.pumpAndSettle();
    await t.enterText(find.widgetWithText(TextFormField, 'Amount'), '500');
    await t.tap(find.widgetWithText(FilledButton, 'Continue'));
    await t.pumpAndSettle();
    expect(find.textContaining('0712 345 678'), findsWidgets, reason: 'the prompt goes to the account number');
    expect(find.textContaining('the full amount'), findsOneWidget, reason: 'top-ups are not taxed');
    expect(find.textContaining('excise'), findsNothing);
    await t.tap(find.widgetWithText(FilledButton, 'Send M-Pesa prompt'));
    await t.pump();
    await t.pump();
    expect(find.text('Approve the M-Pesa prompt on your phone.'), findsOneWidget);
    expect(backend.balance, 0, reason: 'nothing is credited before confirmation');
    expect(h.haptics.events, isNot(contains(HapticEvent.confirmed)));
    await pumpUntilFound(t, find.text('Credited'), step: const Duration(milliseconds: 500));
    expect(h.haptics.events.where((e) => e == HapticEvent.confirmed), hasLength(1));
    await t.tap(find.widgetWithText(FilledButton, 'Back to wallet'));
    await t.pumpAndSettle();
    expect(find.text('KES 500'), findsWidgets, reason: 'the whole top-up is credited');
  });

  testWidgets('the top-up minimum and its quick pick come from the server: KES 1 while testing', (t) async {
    final backend = FakeBackend(balance: 0)
      ..topUpMin = 1
      ..topUpPendingPolls = 1;
    await pumpApp(t, backend);
    await t.tap(find.text('Wallet').last);
    await t.pumpAndSettle();
    await t.tap(find.widgetWithText(FilledButton, 'Top up'));
    await t.pumpAndSettle();
    expect(find.text('KES 1 to KES 250,000, whole shillings'), findsOneWidget);
    await t.tap(find.widgetWithText(ActionChip, 'KES 1'));
    await t.tap(find.widgetWithText(FilledButton, 'Continue'));
    await t.pumpAndSettle();
    await t.tap(find.widgetWithText(FilledButton, 'Send M-Pesa prompt'));
    await pumpUntilFound(t, find.text('Credited'), step: const Duration(milliseconds: 500));
    expect(backend.balance, 1);
  });

  testWidgets('below the server minimum the form refuses before sending', (t) async {
    await pumpApp(t, FakeBackend());
    await t.tap(find.text('Wallet').last);
    await t.pumpAndSettle();
    await t.tap(find.widgetWithText(FilledButton, 'Top up'));
    await t.pumpAndSettle();
    expect(find.widgetWithText(ActionChip, 'KES 1'), findsNothing, reason: 'no quick pick below the minimum');
    await t.enterText(find.widgetWithText(TextFormField, 'Amount'), '1');
    await t.tap(find.widgetWithText(FilledButton, 'Continue'));
    await t.pumpAndSettle();
    expect(find.text('Enter KES 10 to KES 250,000.'), findsOneWidget);
  });

  testWidgets("draws show the player's own periods, counted from their first play", (t) async {
    final backend = FakeBackend()
      ..periods = [
        {'draw_type': 'daily', 'start': '2026-10-30', 'end': '2026-10-30', 'drawn_on': '2026-10-31'},
        {'draw_type': 'weekly', 'start': '2026-10-29', 'end': '2026-11-04', 'drawn_on': '2026-11-05'},
        {'draw_type': 'monthly', 'start': '2026-10-01', 'end': '2026-10-30', 'drawn_on': '2026-10-31'},
        {'draw_type': 'quarterly', 'start': '2026-10-01', 'end': '2026-12-29', 'drawn_on': '2026-12-30'},
      ];
    await pumpApp(t, backend);
    await t.tap(find.text('Draws').last);
    await t.pumpAndSettle();
    expect(find.text('Your draw periods'), findsOneWidget);
    expect(find.text('29 Oct – 4 Nov 2026 · drawn Thu 5 Nov'), findsOneWidget);
    expect(find.text('1 Oct – 29 Dec 2026 · drawn Wed 30 Dec'), findsOneWidget);
  });

  testWidgets('before the first play the draws say periods start with it', (t) async {
    await pumpApp(t, FakeBackend());
    await t.tap(find.text('Draws').last);
    await t.pumpAndSettle();
    expect(
      find.textContaining('Your weekly, monthly and quarterly periods start on the day of your first play'),
      findsOneWidget,
    );
  });

  testWidgets('phones get a bottom bar; desktops an extended rail and a capped, two-column Play', (t) async {
    await pumpApp(t, FakeBackend());
    expect(find.byType(NavigationBar), findsOneWidget);
    expect(find.byType(NavigationRail), findsNothing);
  });

  testWidgets('desktop layout', (t) async {
    await pumpApp(t, FakeBackend(), size: const Size(1440, 900));
    expect(find.byType(NavigationBar), findsNothing);
    expect(t.widget<NavigationRail>(find.byType(NavigationRail)).extended, isTrue);
    await t.tap(find.text('Play').last);
    await t.pumpAndSettle();
    expect(t.takeException(), isNull);
    expect(find.text('Play for KES 100'), findsOneWidget);
  });

  testWidgets('tablet layout uses a labelled rail', (t) async {
    await pumpApp(t, FakeBackend(), size: const Size(820, 1180));
    expect(t.widget<NavigationRail>(find.byType(NavigationRail)).extended, isFalse);
  });

  for (final screen in ['Home', 'Play', 'Draws', 'Wallet', 'Account']) {
    testWidgets('$screen works at 200% text on a small phone', (t) async {
      await pumpApp(t, FakeBackend(), size: const Size(360, 740), textScale: 2);
      await t.tap(find.text(screen).last);
      await t.pumpAndSettle();
      expect(t.takeException(), isNull, reason: 'no overflow');
    });
  }

  testWidgets('the play button can be used from the keyboard', (t) async {
    await pumpApp(t, FakeBackend(), size: const Size(1440, 900));
    await t.tap(find.text('Play').last);
    await t.pumpAndSettle();
    final button = find.widgetWithText(FilledButton, 'Play for KES 100');
    t.widget<FilledButton>(button).focusNode?.requestFocus();
    Focus.of(t.element(find.descendant(of: button, matching: find.byType(Text)).first)).requestFocus();
    await t.pump();
    await t.sendKeyEvent(LogicalKeyboardKey.enter);
    await t.pumpAndSettle();
    expect(find.text('Play for KES 100?'), findsOneWidget);
  });

  testWidgets('the play button tells screen readers the price and the plays left', (t) async {
    final s = t.ensureSemantics();
    await pumpApp(t, FakeBackend());
    await goToPlay(t);
    expect(find.bySemanticsLabel(RegExp(RegExp.escape('Play for KES 100. 4 of 4 plays left today.'))), findsOneWidget);
    s.dispose();
  });

  testWidgets('with wheel haptics off the play gives no vibration but works the same', (t) async {
    final backend = FakeBackend()..spins.add(5);
    final h = await pumpApp(t, backend, hapticsOn: false);
    await goToPlay(t);
    await playOnce(t);
    await pumpUntilFound(t, find.text('Play recorded'));
    expect(h.haptics.events, isEmpty);
    await t.pumpAndSettle();
  });

  testWidgets('with reduced motion the result appears without a spin', (t) async {
    final backend = FakeBackend()..spins.add(8);
    await pumpApp(t, backend, reduceMotion: true);
    await goToPlay(t);
    await playOnce(t);
    await t.pump();
    await t.pump(const Duration(milliseconds: 500));
    expect(find.text('Play recorded'), findsOneWidget);
  });

  testWidgets('when the session ends the player is sent to sign in, with an explanation', (t) async {
    final backend = FakeBackend();
    await pumpApp(t, backend);
    backend.access = 'rotated-elsewhere'; // the token we hold is no longer valid, and refresh will fail
    backend.refreshFails = true;
    await t.tap(find.text('Wallet').last);
    await t.pumpAndSettle();
    expect(find.text('Your session ended. Sign in again to continue.'), findsOneWidget);
  });

  testWidgets('the withdrawal range comes from the server: a KES 20 minimum shows as a quick pick', (t) async {
    final b = FakeBackend(balance: 5000)..withdrawBounds = {'min_kes': 20, 'max_kes': 70000};
    final h = await pumpApp(t, b);
    h.container.read(routerProvider).go('/wallet/withdraw');
    await t.pumpAndSettle();
    expect(find.text('KES 20'), findsOneWidget);
    expect(find.text('KES 100'), findsOneWidget);
  });

  testWidgets('while the server has no M-Pesa payouts the withdraw screen says so instead of a form', (t) async {
    final b = FakeBackend(balance: 5000)..withdrawBounds = null;
    final h = await pumpApp(t, b);
    h.container.read(routerProvider).go('/wallet/withdraw');
    await t.pumpAndSettle();
    expect(find.text('Withdrawals are not available right now'), findsOneWidget);
    expect(find.text('Continue'), findsNothing);
  });

  testWidgets('a strip says when the server cannot be reached, and Retry clears it once it can', (t) async {
    final b = FakeBackend(balance: 1250);
    final h = await pumpApp(t, b);
    expect(find.text("Can't reach the server. What you see may be out of date."), findsNothing);
    b.offline = true;
    h.container.invalidate(walletStatementProvider);
    await t.pumpAndSettle();
    expect(find.text("Can't reach the server. What you see may be out of date."), findsOneWidget);
    b.offline = false;
    await t.tap(find.text('Retry'));
    await t.pumpAndSettle();
    expect(find.text("Can't reach the server. What you see may be out of date."), findsNothing);
    expect(find.text('KES 1,250'), findsWidgets);
  });

  testWidgets(
    'with too little in the wallet, a play is paid from M-Pesa: the prompt is confirmed, then the wheel spins',
    (t) async {
      final b = FakeBackend(balance: 40)..spins.add(3);
      await pumpApp(t, b);
      await t.tap(find.text('Play').last);
      await t.pumpAndSettle();
      expect(find.text('Pay KES 100 with M-Pesa and spin'), findsOneWidget);
      expect(find.text('Play for KES 100'), findsNothing);
      await t.tap(find.text('Pay KES 100 with M-Pesa and spin'));
      await t.pumpAndSettle();
      expect(find.text('Charged to'), findsOneWidget);
      await t.tap(find.widgetWithText(FilledButton, 'Pay KES 100 and spin'));
      await t.pump();
      expect(find.textContaining('Approve the M-Pesa prompt'), findsOneWidget);
      await pumpUntilFound(t, find.text('Play recorded'));
      final topUps = b.requests.where((r) => r.method == 'POST' && r.url.path == '/v1/payments/topups').toList();
      expect(topUps, hasLength(1));
      expect(topUps.single.body, contains('"amount_kes":100'));
      expect(b.requests.where((r) => r.method == 'POST' && r.url.path == '/v1/plays'), hasLength(1));
      expect(b.balance, 40, reason: 'the KES 100 came from the phone and went into the play; the wallet is as it was');
      await t.pumpAndSettle();
    },
  );

  testWidgets('when M-Pesa does not pay, nothing is played and the player is told', (t) async {
    final b = FakeBackend(balance: 0)..topUpPendingPolls = null;
    await pumpApp(t, b);
    await t.tap(find.text('Play').last);
    await t.pumpAndSettle();
    await t.tap(find.text('Pay KES 100 with M-Pesa and spin'));
    await t.pumpAndSettle();
    await t.tap(find.widgetWithText(FilledButton, 'Pay KES 100 and spin'));
    await t.pump();
    final id = b.topUps.keys.single;
    b.topUps[id] = {...b.topUps[id]!, 'status': 'failed'};
    await pumpUntilFound(t, find.textContaining('Nothing was played'), max: 200);
    expect(b.requests.where((r) => r.method == 'POST' && r.url.path == '/v1/plays'), isEmpty);
    await t.pumpAndSettle();
  });
}
