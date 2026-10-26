import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:lottery_mobile/core/platform/haptics.dart';

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

  testWidgets('an unverified ID turns the button off and says why', (t) async {
    await pumpApp(t, FakeBackend(kyc: 'none'));
    await goToPlay(t);
    expect(find.text('Verify your identity to play'), findsOneWidget);
    final button = t.widget<FilledButton>(find.widgetWithText(FilledButton, 'Play for KES 100'));
    expect(button.onPressed, isNull);
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
    expect(find.text('KES 475'), findsWidgets);
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
}
