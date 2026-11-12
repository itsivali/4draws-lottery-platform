import 'package:flutter_test/flutter_test.dart';
import 'package:lottery_mobile/app/router.dart';

import '../support/app_harness.dart';
import '../support/fake_backend.dart';
import '../support/fonts.dart';

/// What Google Play's pre-launch report checks on every screen it reaches (its accessibility scanner): touch targets
/// of at least 48 dp, a label on everything that can be tapped, and readable text contrast. Run here on a phone-sized
/// screen for each of the app's screens, so a failure is found before Play finds it.
void main() {
  setUpAll(loadAppFonts);

  Future<void> check(WidgetTester t, String where) async {
    await t.pumpAndSettle();
    await expectLater(t, meetsGuideline(androidTapTargetGuideline), reason: '$where: a touch target under 48 dp');
    await expectLater(t, meetsGuideline(labeledTapTargetGuideline), reason: '$where: something tappable has no label');
    // contrast is measured on the screen's own pixels: at the harness's 2x density the checker halves the picture
    // first, which blurs thin text into greys and reports contrast the screen does not have
    final logical = t.view.physicalSize / t.view.devicePixelRatio;
    t.view.devicePixelRatio = 1;
    t.view.physicalSize = logical;
    await t.pumpAndSettle();
    await expectLater(t, meetsGuideline(textContrastGuideline), reason: '$where: text contrast');
  }

  testWidgets('signed out: sign in and create account', (t) async {
    final handle = t.ensureSemantics();
    await pumpApp(t, FakeBackend(), signedIn: false);
    await check(t, 'sign in');
    await t.tap(find.text('Create account').first);
    await check(t, 'create account');
    handle.dispose();
  });

  const screens = {
    '/home': 'home',
    '/play': 'play',
    '/draws': 'draws',
    '/wallet': 'wallet',
    '/wallet/top-up': 'top up',
    '/wallet/withdraw': 'withdraw',
    '/account': 'account',
    '/account/details': 'your details',
    '/account/verify': 'identity verification',
    '/account/limits': 'limits, breaks and self-exclusion',
    '/account/devices': 'devices',
    '/account/sign-ins': 'sign-in methods',
    '/account/password': 'change password',
    '/account/email': 'email address',
    '/account/how-it-works': 'how it works',
    '/account/fairness': 'fairness',
    '/account/data': 'your data',
    '/account/close': 'close account',
  };

  for (final MapEntry(key: path, value: name) in screens.entries) {
    testWidgets('signed in: $name', (t) async {
      final handle = t.ensureSemantics();
      final h = await pumpApp(t, FakeBackend());
      h.container.read(routerProvider).go(path);
      await check(t, name);
      handle.dispose();
    });
  }
}
