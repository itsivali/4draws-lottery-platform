import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../../app/theme/tokens.dart';
import '../../../core/formatting/money.dart';
import '../../../core/widgets/app_card.dart';
import '../../../core/widgets/content_frame.dart';
import '../../play/application/play_providers.dart';

/// The rules as the server applies them. Numbers that the server publishes (the price) are read from it; values the
/// operator has not published (the chance table, prize shares) are not stated.
class HowItWorksScreen extends ConsumerWidget {
  const HowItWorksScreen({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final price = ref.watch(todayProvider).value?.priceKes;
    final t = Theme.of(context).textTheme;
    Widget section(String title, List<String> lines) => AppCard(
      title: title,
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          for (final l in lines)
            Padding(
              padding: const EdgeInsets.only(bottom: Space.sm),
              child: Text(l, style: t.bodyMedium),
            ),
        ],
      ),
    );
    return PageScaffold(
      title: 'How it works',
      children: [
        section('Plays', [
          'A play costs ${price == null ? 'the price shown on Play' : formatKes(price)}, paid from your wallet.',
          'You can make up to 4 plays a day. The day runs from midnight to midnight Nairobi time.',
          'Each play is one spin over the slots on the wheel. The server picks the slot; the wheel only shows it.',
        ]),
        section('Chances', [
          "Your plays earn draw chances for the day; the Play screen shows today's count as the server reports it.",
          "A spin on a lucky slot (marked with a star) makes the day lucky, which doubles that day's chances.",
          'The wheel itself never pays cash.',
        ]),
        section('Draws and prizes', [
          'Each chance is one entry in every daily, weekly, monthly and quarterly draw whose period includes that day.',
          'Winners are drawn from a seed committed in advance and a public randomness beacon, then approved by two officials.',
          'Prizes are paid to your M-Pesa number. Tax is withheld as the law requires.',
        ]),
        section('Fairness', [
          'Before any spin, the server publishes the fingerprint of a secret seed. When you reveal it, you can recompute every '
              'spin made with it — on this device, in Fairness.',
        ]),
        section('Who can play', [
          'You must be 18 or older and verify your Kenyan national ID. Your number is confirmed by your first M-Pesa top-up.',
        ]),
      ],
    );
  }
}
