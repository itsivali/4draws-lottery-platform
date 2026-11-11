import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../../app/theme/tokens.dart';
import '../../../core/formatting/money.dart';
import '../../../core/widgets/app_card.dart';
import '../../../core/widgets/content_frame.dart';
import '../../play/application/play_providers.dart';

/// The rules as the server applies them. Numbers that the server publishes (the price, the daily cap) are read from
/// it; values the operator has not published (the chance table, prize shares) are not stated.
class HowItWorksScreen extends ConsumerWidget {
  const HowItWorksScreen({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final today = ref.watch(todayProvider).value;
    final price = today?.priceKes;
    final cap = today == null ? null : today.plays + today.playsLeft;
    final t = Theme.of(context).textTheme;
    final c = context.colors;
    Widget section(IconData icon, String title, List<String> lines) => AppCard(
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Row(
            children: [
              Container(
                width: 36,
                height: 36,
                decoration: BoxDecoration(color: c.primaryContainer, borderRadius: BorderRadius.circular(Radii.sm)),
                child: ExcludeSemantics(child: Icon(icon, size: 20, color: c.onPrimaryContainer)),
              ),
              const SizedBox(width: Space.md),
              Expanded(
                child: Semantics(header: true, child: Text(title, style: t.titleMedium)),
              ),
            ],
          ),
          const SizedBox(height: Space.md),
          for (final (i, l) in lines.indexed)
            Padding(
              padding: EdgeInsets.only(bottom: i == lines.length - 1 ? 0 : Space.sm),
              child: Text(l, style: t.bodyMedium),
            ),
        ],
      ),
    );
    return PageScaffold(
      title: 'How it works',
      children: [
        section(Icons.play_arrow_rounded, 'Plays', [
          'A play costs ${price == null ? 'the price shown on Play' : formatKes(price)}, paid from your wallet.',
          'You can make up to ${cap ?? 'a few'} plays a day. The day runs from midnight to midnight Nairobi time.',
          'Each play is one spin over the slots on the wheel. The server picks the slot; the wheel only shows it.',
        ]),
        section(Icons.confirmation_number_outlined, 'Chances', [
          "Your plays earn draw chances for the day; the Play screen shows today's count as the server reports it.",
          "A spin on a lucky slot (marked with a star) makes the day lucky, which doubles that day's chances.",
          'The wheel itself never pays cash.',
        ]),
        section(Icons.event_rounded, 'Draws and prizes', [
          "Each chance is one entry in the day's draw and in your own weekly, monthly and quarterly draws.",
          'Your weekly, monthly and quarterly periods run 7, 30 and 90 days back to back from the day of your first '
              'play; each is drawn the day after it ends, with the other players whose period ends that day.',
          'Winners are drawn from a seed committed in advance and a public randomness beacon, then approved by two officials.',
          'Prizes are paid to your M-Pesa number. Tax is withheld as the law requires.',
        ]),
        section(Icons.verified_rounded, 'Fairness', [
          'Before any spin, the server publishes the fingerprint of a secret seed. When you reveal it, you can recompute every '
              'spin made with it — on this device, in Fairness.',
        ]),
        section(Icons.badge_rounded, 'Who can play', [
          'You must be 18 or older and verify your Kenyan national ID. Your number is confirmed by your first M-Pesa top-up.',
        ]),
      ],
    );
  }
}
