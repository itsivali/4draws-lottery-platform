import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import '../../../app/theme/tokens.dart';
import '../../../core/formatting/money.dart';
import '../../../core/platform/preferences.dart';
import '../../../core/widgets/announce.dart';
import '../../../core/widgets/app_card.dart';
import '../../../core/widgets/async_view.dart';
import '../../../core/widgets/busy_button.dart';
import '../../../core/widgets/confirm_sheet.dart';
import '../../../core/widgets/content_frame.dart';
import '../../../core/widgets/failure_text.dart';
import '../../../core/widgets/skeleton.dart';
import '../../account/application/account_providers.dart';
import '../../account/domain/account_models.dart';
import '../../wallet/application/wallet_providers.dart';
import '../../wallet/domain/wallet_models.dart';
import '../application/play_controller.dart';
import '../application/play_providers.dart';
import '../application/spin_haptics.dart';
import '../domain/play_eligibility.dart';
import '../domain/play_models.dart';
import '../domain/wheel_geometry.dart';
import 'play_copy.dart';
import 'widgets/lottery_wheel.dart';
import 'widgets/play_receipt_sheet.dart';
import 'widgets/play_status_panel.dart';

class PlayScreen extends ConsumerStatefulWidget {
  const PlayScreen({super.key});

  @override
  ConsumerState<PlayScreen> createState() => _PlayScreenState();
}

class _PlayScreenState extends ConsumerState<PlayScreen> {
  late final SpinHaptics _haptics = SpinHaptics(ref.read(hapticsProvider));

  WheelSpin _spinFor(PlayFlow f) => switch (f.stage) {
    PlayStage.idle => const WheelSpin.idle(),
    PlayStage.submitting => WheelSpin(spinId: f.key),
    PlayStage.landing || PlayStage.revealed => WheelSpin(
      spinId: f.key,
      slot: f.result!.play.spin.slot,
      geometry: WheelGeometry(slots: f.result!.play.spin.slots, luckySlots: f.result!.play.spin.luckySlots),
      within: landingFraction(f.result!.play.ticketId),
    ),
    PlayStage.refused || PlayStage.uncertain => WheelSpin(spinId: f.key, abort: true),
  };

  Future<void> _play(Today today, WalletStatement wallet) async {
    final ok = await confirmAction(
      context,
      title: 'Play for ${formatKes(today.priceKes)}?',
      facts: [
        ('Price', formatKes(today.priceKes)),
        ('Paid from', 'Your wallet (${formatKes(wallet.balanceKes)})'),
        ('Plays left today after this', '${today.playsLeft - 1} of 4'),
        ('What you get', 'One spin. A lucky slot doubles today’s draw chances.'),
      ],
      note: 'No cash is paid from the wheel. Prizes come only from the draws.',
      confirmLabel: 'Play for ${formatKes(today.priceKes)}',
    );
    if (ok && mounted) await ref.read(playControllerProvider.notifier).start();
  }

  void _announce(PlayFlow? prev, PlayFlow next) {
    if (prev?.stage == next.stage) return;
    switch (next.stage) {
      case PlayStage.revealed:
        final d = describeResult(next.result!);
        announce(context, '${d.heading}. ${d.detail}');
      case PlayStage.refused:
        announce(context, describeFailure(next.failure!).title);
      case PlayStage.uncertain:
        announce(context, 'Checking whether your play went through.');
      case PlayStage.idle || PlayStage.submitting || PlayStage.landing:
        break;
    }
  }

  @override
  Widget build(BuildContext context) {
    ref.listen(playControllerProvider, _announce);
    final flow = ref.watch(playControllerProvider);
    final today = ref.watch(todayProvider);
    final account = ref.watch(accountProvider);
    final wallet = ref.watch(walletStatementProvider);
    final ctl = ref.read(playControllerProvider.notifier);

    // the wheel at rest shows the server's own wheel once a play today has described it
    final last = (flow.result?.play ?? today.value?.tickets.lastOrNull)?.spin;
    final resting = last == null ? null : WheelGeometry(slots: last.slots, luckySlots: last.luckySlots);

    final wheel = LotteryWheel(
      spin: _spinFor(flow),
      restingGeometry: resting,
      haptics: _haptics,
      onSettled: ctl.landed,
    );
    final panel = _PlayPanel(
      flow: flow,
      today: today,
      account: account,
      wallet: wallet,
      onPlay: _play,
      onRetry: () {
        ref
          ..invalidate(todayProvider)
          ..invalidate(accountProvider)
          ..invalidate(walletStatementProvider);
      },
    );

    final wide = widthClassOf(context) == WidthClass.expanded;
    return Scaffold(
      appBar: AppBar(title: Semantics(header: true, child: const Text('Play'))),
      body: SafeArea(
        top: false,
        child: wide
            ? ContentFrame(
                wide: true,
                child: Padding(
                  padding: const EdgeInsets.all(Space.xl),
                  child: Row(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      Expanded(
                        flex: 5,
                        child: Center(
                          child: Padding(padding: const EdgeInsets.all(Space.lg), child: wheel),
                        ),
                      ),
                      const SizedBox(width: Space.xxl),
                      Expanded(flex: 4, child: SingleChildScrollView(child: panel)),
                    ],
                  ),
                ),
              )
            : ListView(
                padding: const EdgeInsets.fromLTRB(Space.lg, Space.sm, Space.lg, Space.xxl),
                children: [
                  ContentFrame(
                    child: Column(
                      crossAxisAlignment: CrossAxisAlignment.stretch,
                      children: [
                        Center(
                          child: ConstrainedBox(
                            constraints: BoxConstraints(
                              maxHeight: MediaQuery.sizeOf(context).height * 0.46,
                              maxWidth: 420,
                            ),
                            child: AspectRatio(aspectRatio: 1, child: wheel),
                          ),
                        ),
                        const SizedBox(height: Space.xl),
                        panel,
                      ],
                    ),
                  ),
                ],
              ),
      ),
    );
  }
}

class _PlayPanel extends ConsumerWidget {
  const _PlayPanel({
    required this.flow,
    required this.today,
    required this.account,
    required this.wallet,
    required this.onPlay,
    required this.onRetry,
  });

  final PlayFlow flow;
  final AsyncValue<Today> today;
  final AsyncValue<Account> account;
  final AsyncValue<WalletStatement> wallet;
  final Future<void> Function(Today, WalletStatement) onPlay;
  final VoidCallback onRetry;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final ctl = ref.read(playControllerProvider.notifier);
    final error = today.error ?? account.error ?? wallet.error;
    if (error != null && !(today.hasValue && account.hasValue && wallet.hasValue)) {
      return ErrorNotice(error: error, onRetry: onRetry, what: 'your play day');
    }
    if (!(today.hasValue && account.hasValue && wallet.hasValue)) {
      return const Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          Skeleton(height: 72, radius: Radii.lg),
          SizedBox(height: Space.lg),
          Skeleton(height: 52, radius: Radii.md),
        ],
      );
    }
    final t = today.requireValue, a = account.requireValue, w = wallet.requireValue;
    // the result's fresh day is shown once the wheel has stopped, never while it still turns
    final revealed = flow.stage == PlayStage.revealed;
    final day = revealed ? flow.result!.today : t;
    final balance = flow.result?.balanceKes ?? w.balanceKes;
    final block = playBlock(
      account: a,
      today: day,
      wallet: WalletStatement(balanceKes: balance, debtKes: w.debtKes, transactions: const []),
    );
    final c = context.colors;
    final text = Theme.of(context).textTheme;

    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        _DayStrip(today: day),
        const SizedBox(height: Space.lg),
        PlayStatusPanel(flow: flow, onDone: ctl.dismiss, onCheckAgain: ctl.checkAgain, onForget: ctl.forget),
        if (flow.stage != PlayStage.idle) const SizedBox(height: Space.lg),
        if (block != null && !flow.busy) ...[
          _BlockNotice(block: block, priceKes: day.priceKes),
          const SizedBox(height: Space.lg),
        ],
        BusyButton(
          label: 'Play for ${formatKes(day.priceKes)}',
          icon: Icons.donut_large_rounded,
          busy: flow.stage == PlayStage.submitting,
          semanticsLabel: 'Play for ${formatKes(day.priceKes)}. ${day.playsLeft} of 4 plays left today.',
          onPressed: block != null || flow.busy
              ? null
              : () => onPlay(day, WalletStatement(balanceKes: balance, debtKes: w.debtKes, transactions: const [])),
        ),
        const SizedBox(height: Space.sm),
        Text('Balance ${formatKes(balance)}', style: text.bodySmall, textAlign: TextAlign.center),
        const SizedBox(height: Space.xl),
        const _WhatAPlayDoes(),
        if (day.tickets.isNotEmpty) ...[
          const SizedBox(height: Space.lg),
          AppCard(
            title: "Today's plays",
            padding: const EdgeInsets.fromLTRB(Space.lg, Space.lg, Space.lg, Space.sm),
            child: Column(
              children: [
                for (final p in day.tickets.reversed)
                  ListTile(
                    contentPadding: EdgeInsets.zero,
                    leading: ExcludeSemantics(
                      child: Icon(
                        p.spin.lucky ? Icons.star_rounded : Icons.donut_large_rounded,
                        color: p.spin.lucky ? c.goldText : c.inkMuted,
                      ),
                    ),
                    title: Text('Play ${p.seq} · slot ${p.spin.slot}${p.spin.lucky ? ' (lucky)' : ''}'),
                    subtitle: Text(formatKes(p.priceKes)),
                    trailing: const Icon(Icons.chevron_right_rounded),
                    onTap: () => showPlayReceipt(context, p),
                  ),
              ],
            ),
          ),
        ],
      ],
    );
  }
}

class _DayStrip extends StatelessWidget {
  const _DayStrip({required this.today});

  final Today today;

  @override
  Widget build(BuildContext context) {
    final t = Theme.of(context).textTheme;
    final c = context.colors;
    Widget cell(String label, String value, {Widget? badge}) => Expanded(
      child: MergeSemantics(
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Text(label, style: t.bodySmall),
            const SizedBox(height: Space.xxs),
            Row(
              children: [
                Flexible(child: Text(value, style: t.titleLarge)),
                if (badge != null) ...[const SizedBox(width: Space.xs), badge],
              ],
            ),
          ],
        ),
      ),
    );
    return AppCard(
      child: Row(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          cell('Price per play', formatKes(today.priceKes)),
          cell('Plays left today', '${today.playsLeft} of 4'),
          cell(
            'Draw chances today',
            '${today.chances}',
            badge: today.lucky
                ? Semantics(
                    label: 'lucky day, doubled',
                    child: ExcludeSemantics(child: Icon(Icons.star_rounded, size: 18, color: c.goldText)),
                  )
                : null,
          ),
        ],
      ),
    );
  }
}

class _BlockNotice extends StatelessWidget {
  const _BlockNotice({required this.block, required this.priceKes});

  final PlayBlock block;
  final int priceKes;

  @override
  Widget build(BuildContext context) {
    final d = describeBlock(block, priceKes: priceKes);
    final t = Theme.of(context).textTheme;
    return Semantics(
      container: true,
      child: Row(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          ExcludeSemantics(child: Icon(Icons.info_rounded, color: context.colors.warning)),
          const SizedBox(width: Space.md),
          Expanded(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Text(d.title, style: t.titleMedium),
                Text(d.body, style: t.bodyMedium),
                if (d.route != null)
                  Padding(
                    padding: const EdgeInsets.only(top: Space.sm),
                    child: OutlinedButton(onPressed: () => context.go(d.route!), child: Text(d.action!)),
                  ),
              ],
            ),
          ),
        ],
      ),
    );
  }
}

class _WhatAPlayDoes extends StatelessWidget {
  const _WhatAPlayDoes();

  @override
  Widget build(BuildContext context) {
    final t = Theme.of(context).textTheme;
    return AppCard(
      title: 'What a play does',
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Text(
            'Each play is one spin over the slots shown on the wheel. The server picks the slot with your provably fair '
            'seed chain; the wheel only shows it.',
            style: t.bodyMedium,
          ),
          const SizedBox(height: Space.sm),
          Text(
            "Your plays earn draw chances for the day. A spin on a lucky slot (marked with a star) doubles today's chances. "
            'Chances enter the daily, weekly, monthly and quarterly draws whose periods include today.',
            style: t.bodyMedium,
          ),
          const SizedBox(height: Space.sm),
          Text(
            'The wheel never pays cash. Prizes come only from the draws.',
            style: t.bodyMedium?.copyWith(fontWeight: FontWeight.w600),
          ),
          Align(
            alignment: Alignment.centerLeft,
            child: TextButton(onPressed: () => context.go('/account/how-it-works'), child: const Text('How it works')),
          ),
        ],
      ),
    );
  }
}
