import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import '../../../app/theme/theme.dart';
import '../../../app/theme/tokens.dart';
import '../../../core/formatting/money.dart';
import '../../../core/formatting/phone.dart';
import '../../../core/platform/preferences.dart';
import '../../../core/widgets/announce.dart';
import '../../../core/widgets/app_card.dart';
import '../../../core/widgets/async_view.dart';
import '../../../core/widgets/busy_button.dart';
import '../../../core/widgets/confirm_sheet.dart';
import '../../../core/widgets/content_frame.dart';
import '../../../core/widgets/failure_text.dart';
import '../../../core/widgets/glow.dart';
import '../../../core/widgets/hero_card.dart';
import '../../../core/widgets/inline_banner.dart';
import '../../../core/widgets/list_row.dart';
import '../../../core/widgets/motion.dart';
import '../../../core/widgets/skeleton.dart';
import '../../../core/widgets/status_pill.dart';
import '../../account/application/account_providers.dart';
import '../../account/domain/account_models.dart';
import '../../wallet/application/money_flows.dart';
import '../../wallet/application/top_up_cooldown.dart';
import '../../wallet/application/wallet_providers.dart';
import '../../wallet/domain/wallet_models.dart';
import '../../wallet/presentation/money_flow_view.dart';
import '../../wallet/presentation/receipt_claim_card.dart';
import '../../wallet/presentation/top_up_cooldown_note.dart';
import '../application/pay_and_play.dart';
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

  /// The wallet cannot cover the price: the price is charged to the player's M-Pesa and the wheel spins once paid.
  Future<void> _payAndPlay(Today today, Account account) async {
    final ok = await confirmAction(
      context,
      title: 'Pay ${formatKes(today.priceKes)} with M-Pesa and spin?',
      facts: [
        ('Price', formatKes(today.priceKes)),
        ('Charged to', formatMsisdn(account.msisdn)),
        ('Plays left today after this', '${today.playsLeft - 1} of ${today.plays + today.playsLeft}'),
        ('What you get', 'One spin. A lucky slot doubles today’s draw chances.'),
      ],
      note:
          'An M-Pesa prompt comes to your phone; enter your PIN there. The wheel spins the moment M-Pesa confirms the '
          'payment. No cash is paid from the wheel. Prizes come only from the draws.',
      confirmLabel: 'Pay ${formatKes(today.priceKes)} and spin',
    );
    if (ok && mounted) ref.read(payAndPlayProvider.notifier).start(today.priceKes);
  }

  Future<void> _play(Today today, WalletStatement wallet) async {
    final ok = await confirmAction(
      context,
      title: 'Play for ${formatKes(today.priceKes)}?',
      facts: [
        ('Price', formatKes(today.priceKes)),
        ('Paid from', 'Your wallet (${formatKes(wallet.balanceKes)})'),
        ('Plays left today after this', '${today.playsLeft - 1} of ${today.plays + today.playsLeft}'),
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

    // the wheel at rest shows the server's own wheel: from the last play, else from today's odds, so the numbers show
    // before the first spin (owner, 2026-10-30)
    final last = (flow.result?.play ?? today.value?.tickets.lastOrNull)?.spin;
    final day = today.value;
    final resting = last != null
        ? WheelGeometry(slots: last.slots, luckySlots: last.luckySlots)
        : day?.slots != null && day!.luckySlots <= day.slots!
        ? WheelGeometry(slots: day.slots!, luckySlots: day.luckySlots)
        : null;

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
      onPayAndPlay: _payAndPlay,
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
                        child: _Stage(
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
                padding: const EdgeInsets.fromLTRB(Space.lg, Space.lg, Space.lg, Space.xxl),
                children: [
                  ContentFrame(
                    child: Column(
                      crossAxisAlignment: CrossAxisAlignment.stretch,
                      children: [
                        _Stage(
                          child: Center(
                            child: ConstrainedBox(
                              constraints: BoxConstraints(
                                maxHeight: MediaQuery.sizeOf(context).height * 0.42,
                                maxWidth: 400,
                              ),
                              child: AspectRatio(aspectRatio: 1, child: wheel),
                            ),
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

/// The wheel's stage: the app's second hero surface, the dusk end of the sunset, against which the segments, the rim
/// and the lucky star all stand out. Since the redesign of 2026-11-11 the wheel sits in a dotted dial, like a timer's
/// bezel, over a soft coral glow.
class _Stage extends StatelessWidget {
  const _Stage({required this.child});

  final Widget child;

  @override
  Widget build(BuildContext context) {
    final c = context.colors;
    return HeroCard(
      stage: true,
      radius: Radii.xxl,
      padding: const EdgeInsets.all(Space.md),
      child: Stack(
        alignment: Alignment.center,
        children: [
          Positioned.fill(
            child: ExcludeSemantics(
              child: DecoratedBox(
                decoration: BoxDecoration(gradient: Gradients.blob(c.heroGlow, radius: 0.5, alpha: 0.38)),
              ),
            ),
          ),
          DottedDial(child: child),
        ],
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
    required this.onPayAndPlay,
    required this.onRetry,
  });

  final PlayFlow flow;
  final AsyncValue<Today> today;
  final AsyncValue<Account> account;
  final AsyncValue<WalletStatement> wallet;
  final Future<void> Function(Today, WalletStatement) onPlay;
  final Future<void> Function(Today, Account) onPayAndPlay;
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
          Skeleton(height: 88, radius: Radii.lg),
          SizedBox(height: Space.lg),
          Skeleton(height: 52, radius: Radii.md),
        ],
      );
    }
    final t = today.requireValue, a = account.requireValue, w = wallet.requireValue;
    // the result's fresh day is shown once the wheel has stopped, never while it still turns
    final revealed = flow.stage == PlayStage.revealed;
    final day = revealed ? flow.result!.today : t;
    // the wallet is re-read the moment a play is charged, so it is the one balance to show (a top-up made while
    // the wheel turned is in it; the result's own figure is shown in the result panel)
    final balance = w.balanceKes;
    final cap = day.plays + day.playsLeft;
    final block = playBlock(
      account: a,
      today: day,
      wallet: WalletStatement(balanceKes: balance, debtKes: w.debtKes, transactions: const []),
    );
    final text = Theme.of(context).textTheme;
    // only the balance stands in the way: the price can be charged to the player's M-Pesa instead
    final payInstead = block?.reason == PlayBlockReason.lowBalance;
    final paying = ref.watch(payAndPlayProvider);
    final topUp = ref.watch(topUpFlowProvider);
    final busy = flow.busy || paying == PayAndPlayStage.paying;

    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        _DayStrip(today: day),
        const SizedBox(height: Space.lg),
        PlayStatusPanel(
          flow: flow,
          onDone: ctl.dismiss,
          onCheckAgain: ctl.checkAgain,
          onForget: ctl.forget,
          onSetAside: ctl.setAside,
        ),
        if (flow.stage != PlayStage.idle) const SizedBox(height: Space.lg),
        if (paying != PayAndPlayStage.idle) ...[
          AppCard(
            child: SettlementStatusView<TopUp>(
              flow: topUp.flow,
              pendingText: 'Approve the M-Pesa prompt on your phone. The wheel spins once M-Pesa confirms it.',
              describe: (t) => switch (t.status) {
                TopUpStatus.credited => (StatusTone.success, 'Paid', 'M-Pesa confirmed it. Spinning.'),
                TopUpStatus.failed => (StatusTone.error, 'Not paid', 'M-Pesa did not pay it. Nothing was played.'),
                TopUpStatus.unconfirmed => (
                  StatusTone.warning,
                  'Not confirmed',
                  'M-Pesa has not confirmed it to us. If it took the money, we credit your wallet once we have checked '
                      'the statement. Nothing was played.',
                ),
                TopUpStatus.reversed => (StatusTone.error, 'Reversed', 'M-Pesa took it back. Nothing was played.'),
                TopUpStatus.pending => (StatusTone.pending, 'Waiting', ''),
              },
              below: (x) => x.status == TopUpStatus.unconfirmed || x.status == TopUpStatus.failed
                  ? ReceiptClaimCard(topUp: x, compact: x.status == TopUpStatus.failed, onChanged: topUp.flow.refresh)
                  : null,
              onDone: ref.read(payAndPlayProvider.notifier).dismiss,
            ),
          ),
          const SizedBox(height: Space.lg),
        ],
        if (block != null && !busy && !payInstead) ...[
          _BlockNotice(block: block, priceKes: day.priceKes, dailyCap: cap),
          const SizedBox(height: Space.lg),
        ],
        // paying with M-Pesa waits out the top-up cooldown, and says how long (owner, 2026-10-30)
        TopUpCooldownGate(
          readyAt: payInstead && !busy ? ref.watch(topUpCooldownProvider) : null,
          what: 'pay with M-Pesa',
          builder: (context, ready, note) => Column(
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              PressScale(
                enabled: (block == null || payInstead) && !busy && ready,
                child: BusyButton(
                  label: payInstead
                      ? 'Pay ${formatKes(day.priceKes)} with M-Pesa and spin'
                      : 'Play for ${formatKes(day.priceKes)}',
                  icon: payInstead ? Icons.phone_iphone_rounded : Icons.donut_large_rounded,
                  busy: flow.stage == PlayStage.submitting || paying == PayAndPlayStage.paying,
                  semanticsLabel: payInstead
                      ? 'Pay ${formatKes(day.priceKes)} with M-Pesa and spin. ${day.playsLeft} of $cap plays left today.'
                      : 'Play for ${formatKes(day.priceKes)}. ${day.playsLeft} of $cap plays left today.',
                  onPressed: busy || !ready || (block != null && !payInstead)
                      ? null
                      : payInstead
                      ? () => onPayAndPlay(day, a)
                      : () => onPlay(
                          day,
                          WalletStatement(balanceKes: balance, debtKes: w.debtKes, transactions: const []),
                        ),
                ),
              ),
              if (note != null) ...[const SizedBox(height: Space.sm), note],
            ],
          ),
        ),
        const SizedBox(height: Space.sm),
        Text(
          payInstead
              ? 'Balance ${formatKes(balance)} · the price is charged to your M-Pesa; your balance stays in your wallet'
              : 'Balance ${formatKes(balance)}',
          style: text.bodySmall,
          textAlign: TextAlign.center,
        ),
        if (payInstead && !busy)
          TextButton(
            style: TextButton.styleFrom(foregroundColor: context.colors.inkMuted),
            onPressed: () => context.go('/wallet/top-up'),
            child: const Text('Top up a larger amount instead'),
          ),
        const SizedBox(height: Space.xl),
        const _WhatAPlayDoes(),
        if (day.tickets.isNotEmpty) ...[
          const SizedBox(height: Space.lg),
          AppCard(
            title: "Today's plays",
            icon: Icons.casino_rounded,
            padding: const EdgeInsets.only(top: Space.lg),
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.stretch,
              children: [
                for (final (i, p) in day.tickets.reversed.indexed) ...[
                  if (i > 0) Divider(indent: Space.lg, endIndent: Space.lg, color: context.colors.border),
                  ListRow(
                    icon: p.spin.lucky ? Icons.star_rounded : Icons.donut_large_rounded,
                    iconColor: p.spin.lucky ? context.colors.goldText : context.colors.inkMuted,
                    title: 'Play ${p.seq} · slot ${p.spin.slot}${p.spin.lucky ? ' (lucky)' : ''}',
                    subtitle: formatKes(p.priceKes),
                    chevron: true,
                    onTap: () => showPlayReceipt(context, p),
                  ),
                ],
              ],
            ),
          ),
        ],
      ],
    );
  }
}

/// Plays left as a ring gauge with the number inside, then the price and the chances under small captions. The
/// "N of M" text is one widget on purpose: it is what the player (and the tests) read as the count.
class _DayStrip extends StatelessWidget {
  const _DayStrip({required this.today});

  final Today today;

  @override
  Widget build(BuildContext context) {
    final t = Theme.of(context).textTheme;
    final c = context.colors;
    final cap = today.plays + today.playsLeft;
    final big = t.headlineMedium?.weighted(FontWeight.w300);
    Widget cell(String label, String value, {Widget? badge}) => Expanded(
      child: MergeSemantics(
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Text(label, style: t.labelMedium?.copyWith(color: c.inkMuted)),
            const SizedBox(height: Space.xxs),
            Row(
              children: [
                Flexible(child: Text(value, style: big)),
                if (badge != null) ...[const SizedBox(width: Space.xs), badge],
              ],
            ),
          ],
        ),
      ),
    );
    return AppCard(
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          Row(
            children: [
              RingGauge(
                value: cap == 0 ? 0 : today.playsLeft / cap,
                child: Text('${today.playsLeft}', style: t.titleLarge?.weighted(FontWeight.w500)),
              ),
              const SizedBox(width: Space.lg),
              Expanded(
                child: MergeSemantics(
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      Text('Plays left today', style: t.labelMedium?.copyWith(color: c.inkMuted)),
                      const SizedBox(height: Space.xxs),
                      Text('${today.playsLeft} of $cap', style: big),
                    ],
                  ),
                ),
              ),
            ],
          ),
          const SizedBox(height: Space.lg),
          Divider(color: c.border),
          const SizedBox(height: Space.lg),
          Row(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              cell('Price per play', formatKes(today.priceKes)),
              const SizedBox(width: Space.lg),
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
        ],
      ),
    );
  }
}

class _BlockNotice extends StatelessWidget {
  const _BlockNotice({required this.block, required this.priceKes, required this.dailyCap});

  final PlayBlock block;
  final int priceKes;
  final int dailyCap;

  @override
  Widget build(BuildContext context) {
    final d = describeBlock(block, priceKes: priceKes, dailyCap: dailyCap);
    return Semantics(
      container: true,
      child: InlineBanner(
        tone: StatusTone.warning,
        icon: Icons.info_rounded,
        title: d.title,
        message: d.body,
        actionLabel: d.action,
        onAction: d.route == null ? null : () => context.go(d.route!),
      ),
    );
  }
}

class _WhatAPlayDoes extends StatelessWidget {
  const _WhatAPlayDoes();

  @override
  Widget build(BuildContext context) {
    final t = Theme.of(context).textTheme;
    final c = context.colors;
    Widget point(IconData icon, String text, {bool strong = false}) => Padding(
      padding: const EdgeInsets.only(bottom: Space.md),
      child: Row(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          ExcludeSemantics(child: Icon(icon, size: 20, color: c.primary)),
          const SizedBox(width: Space.md),
          Expanded(
            child: Text(text, style: strong ? t.bodyMedium?.copyWith(fontWeight: FontWeight.w600) : t.bodyMedium),
          ),
        ],
      ),
    );
    return AppCard(
      title: 'What a play does',
      icon: Icons.help_outline_rounded,
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          point(
            Icons.donut_large_rounded,
            'Each play is one spin over the slots shown on the wheel. The server picks the slot with your provably fair '
            'seed chain; the wheel only shows it.',
          ),
          point(
            Icons.confirmation_number_outlined,
            "Your plays earn draw chances for the day. A spin on a lucky slot (marked with a star) doubles today's "
            'chances. Chances enter the daily, weekly, monthly and quarterly draws whose periods include today.',
          ),
          point(Icons.block_rounded, 'The wheel never pays cash. Prizes come only from the draws.', strong: true),
          Align(
            alignment: Alignment.centerLeft,
            child: TextButton(onPressed: () => context.go('/account/how-it-works'), child: const Text('How it works')),
          ),
        ],
      ),
    );
  }
}
