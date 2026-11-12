import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';

import '../support/app_harness.dart';
import '../support/fake_backend.dart';
import '../support/fonts.dart';

void main() {
  setUpAll(loadAppFonts);

  // owner, 2026-12: on a laptop the sign-in form must sit in the middle of the screen, whole, with nothing to scroll
  testWidgets('with no Google or Apple sign-in on offer it is one column, and still fits', (t) async {
    const size = Size(1342, 600);
    await pumpApp(t, FakeBackend()..providers = {}, signedIn: false, size: size);
    await t.pumpAndSettle();
    final scroll = t.state<ScrollableState>(find.byType(Scrollable).first);
    expect(scroll.position.maxScrollExtent, 0, reason: 'the page scrolls by ${scroll.position.maxScrollExtent}');
    final card = t.getRect(find.ancestor(of: find.text('Good to see you'), matching: find.byType(Material)).first);
    expect(card.width, lessThanOrEqualTo(440));
    expect(card.center.dx, moreOrLessEquals(size.width / 2, epsilon: 0.5));
  });

  testWidgets('a long form (create account) scrolls, with no scroll bar shown', (t) async {
    await pumpApp(t, FakeBackend(), signedIn: false, size: const Size(1342, 631));
    await t.tap(find.text('Create account').first);
    await t.pumpAndSettle();
    final scroll = t.state<ScrollableState>(find.byType(Scrollable).first);
    expect(scroll.position.maxScrollExtent, greaterThan(0), reason: 'the longer form needs scrolling');
    expect(find.byType(Scrollbar), findsNothing);
    expect(find.byType(RawScrollbar), findsNothing);
  });

  testWidgets('on a phone Google and Apple stay under the form', (t) async {
    await pumpApp(t, FakeBackend(), signedIn: false);
    await t.pumpAndSettle();
    final signIn = t.getRect(find.widgetWithText(FilledButton, 'Sign in'));
    final google = t.getRect(find.textContaining('Continue with Google'));
    expect(google.top, greaterThan(signIn.bottom));
  });

  for (final size in const [Size(1342, 600), Size(1366, 657), Size(1440, 900)]) {
    testWidgets('sign-in fits a ${size.width.toInt()}×${size.height.toInt()} window without scrolling, centred', (
      t,
    ) async {
      await pumpApp(t, FakeBackend(), signedIn: false, size: size);
      await t.pumpAndSettle();
      final scroll = t.state<ScrollableState>(find.byType(Scrollable).first);
      expect(scroll.position.maxScrollExtent, 0, reason: 'the page scrolls by ${scroll.position.maxScrollExtent}');
      final card = t.getRect(find.ancestor(of: find.text('Good to see you'), matching: find.byType(Material)).first);
      expect(card.center.dx, moreOrLessEquals(size.width / 2, epsilon: 0.5));
      expect(card.top, greaterThan(0));
      expect(card.bottom, lessThan(size.height));
      expect(find.textContaining('Play responsibly. Lottery plays cost money'), findsOneWidget);
    });
  }
}
