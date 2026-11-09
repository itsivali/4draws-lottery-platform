import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:lottery_mobile/app/router.dart';

import '../support/app_harness.dart';
import '../support/fake_backend.dart';
import '../support/fonts.dart';
import '../support/journeys.dart';

/// ADR 0030: self-excluding signs the player out on every device; the sign-in screen says why, not that the session
/// timed out.
void main() {
  setUpAll(loadAppFonts);

  testWidgets('self-excluding signs out and the sign-in screen says why', (t) async {
    final backend = FakeBackend();
    final h = await pumpApp(t, backend);
    h.container.read(routerProvider).go('/account/limits');
    await t.pumpAndSettle();
    await tapVisible(t, find.widgetWithText(OutlinedButton, '6 months'));
    await t.pumpAndSettle();
    expect(find.text('On every device, straight away.'), findsOneWidget, reason: 'the confirmation says it signs out');
    await t.tap(find.widgetWithText(FilledButton, 'Self-exclude'));
    await t.pumpAndSettle();
    expect(backend.requests.where((r) => r.method == 'POST' && r.url.path == '/v1/rg/self-exclusion').length, 1);
    expect(find.text('You are self-excluded'), findsOneWidget);
    expect(find.text('Your session timed out'), findsNothing);
  });
}
