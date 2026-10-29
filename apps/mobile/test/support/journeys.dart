import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';

import 'app_harness.dart';
import 'fake_backend.dart';

/// The main journey, shared by the VM widget test (test/app/journey_test.dart) and the on-device integration test
/// (integration_test/play_journey_test.dart): sign in, a play with its full spin, a lost answer recovered after
/// leaving and coming back, and a top-up that stays pending until confirmed.
/// Scrolls [f] into view, then taps it.
Future<void> tapVisible(WidgetTester t, Finder f) async {
  await t.ensureVisible(f);
  await t.pumpAndSettle();
  await t.tap(f);
}

Future<void> playJourney(WidgetTester t, {Size? size}) async {
  final backend = FakeBackend(balance: 300)..spins.addAll([4, 0]);
  await pumpApp(t, backend, signedIn: false, size: size ?? const Size(390, 844));

  await t.enterText(find.widgetWithText(TextFormField, 'M-Pesa phone number'), '0712345678');
  await t.enterText(find.widgetWithText(TextFormField, 'Password'), 'correct horse battery');
  await t.tap(find.widgetWithText(FilledButton, 'Sign in'));
  await t.pumpAndSettle();

  await t.tap(find.text('Play').last);
  await t.pumpAndSettle();
  await tapVisible(t, find.widgetWithText(FilledButton, 'Play for KES 100').first);
  await t.pumpAndSettle();
  await t.tap(find.widgetWithText(FilledButton, 'Play for KES 100').last);
  await pumpUntilFound(t, find.text('Play recorded'));
  await tapVisible(t, find.text('Done'));
  await t.pumpAndSettle();

  // the answer to the second play is lost after the server committed it
  backend.loseNextPlayAnswer = true;
  await tapVisible(t, find.widgetWithText(FilledButton, 'Play for KES 100').first);
  await t.pumpAndSettle();
  await t.tap(find.widgetWithText(FilledButton, 'Play for KES 100').last);
  await pumpUntilFound(t, find.text("We're checking whether your play went through"));
  // leave and come back while it is being checked
  await t.tap(find.text('Home').last);
  await t.pump(const Duration(seconds: 2));
  await t.tap(find.text('Play').last);
  await pumpUntilFound(t, find.text("Lucky slot — today's chances are doubled"));
  expect(backend.plays, 2);
  expect(backend.balance, 100);

  await t.tap(find.text('Wallet').last);
  await t.pumpAndSettle();
  await t.tap(find.widgetWithText(FilledButton, 'Top up'));
  await t.pumpAndSettle();
  await t.enterText(find.widgetWithText(TextFormField, 'Amount'), '200');
  await t.tap(find.widgetWithText(FilledButton, 'Continue'));
  await t.pumpAndSettle();
  await t.tap(find.widgetWithText(FilledButton, 'Send M-Pesa prompt'));
  await pumpUntilFound(t, find.textContaining('Approve the M-Pesa prompt on your phone.'));
  expect(backend.balance, 100);
  await pumpUntilFound(t, find.text('Credited'), step: const Duration(milliseconds: 500));
}
