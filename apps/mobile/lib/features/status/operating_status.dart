import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../app/providers.dart';
import '../../core/widgets/inline_banner.dart';
import '../../core/widgets/status_pill.dart';

/// What the service may do with money now (ADR 0027): `sandbox` (test money only), `live`, or `disabled` (stopped).
final operatingModeProvider = FutureProvider<String?>((ref) async {
  final r = await ref.watch(apiClientProvider).get('/v1/status', authenticated: false);
  final mode = r.json['operating_mode'];
  return mode is String ? mode : null;
});

/// Says so when the service is a demonstration or stopped (S8): players must never think test money is real, or that
/// a stopped service is broken. Says nothing when live, or when the status cannot be read.
class OperatingModeBanner extends ConsumerWidget {
  const OperatingModeBanner({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) => switch (ref.watch(operatingModeProvider).value) {
    'sandbox' => const InlineBanner(
      tone: StatusTone.pending,
      icon: Icons.science_outlined,
      title: 'Demo: test money only, no real prizes',
      message:
          '4draws is not licensed yet. Top-ups use M-Pesa’s test system and nothing won can be paid out as real money.',
    ),
    'disabled' => const InlineBanner(
      tone: StatusTone.warning,
      icon: Icons.pause_circle_outline_rounded,
      title: 'Top-ups, plays and withdrawals are paused',
      message: 'Your balance is safe. Try again later.',
    ),
    _ => const SizedBox.shrink(),
  };
}
