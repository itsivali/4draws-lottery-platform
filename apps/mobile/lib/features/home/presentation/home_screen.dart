import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import '../../../app/theme/tokens.dart';
import '../../../core/formatting/dates.dart';
import '../../../core/formatting/money.dart';
import '../../../core/widgets/app_card.dart';
import '../../../core/widgets/async_view.dart';
import '../../../core/widgets/brand_mark.dart';
import '../../../core/widgets/coin.dart';
import '../../../core/widgets/content_frame.dart';
import '../../../core/widgets/glow.dart';
import '../../../core/widgets/inline_banner.dart';
import '../../../core/widgets/motion.dart';
import '../../../core/widgets/reveal.dart';
import '../../../core/widgets/skeleton.dart';
import '../../../core/widgets/status_pill.dart';
import '../../account/application/account_providers.dart';
import '../../account/domain/account_models.dart';
import '../../draws/application/draws_providers.dart';
import '../../draws/presentation/draws_screen.dart';
import '../../play/application/play_providers.dart';
import '../../play/domain/play_models.dart';
import '../../status/operating_status.dart';
import '../../wallet/application/wallet_providers.dart';

class HomeScreen extends ConsumerWidget {
  const HomeScreen({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final wide = widthClassOf(context) == WidthClass.expanded;
    final account = ref.watch(accountProvider);
    final wallet = ref.watch(walletStatementProvider);
    final cards = <Widget>[
      _Greeting(name: account.value?.firstName),
      // only when there is something to say: a demo or a stopped service (S8)
      if (ref.watch(operatingModeProvider).value case 'sandbox' || 'disabled') const OperatingModeBanner(),
      // an account from before sign-up asked for them is asked until it gives them (owner, 2026-10-31)
      if (account.value?.needsIdentity ?? false)
        InlineBanner(
          tone: StatusTone.warning,
          icon: Icons.badge_outlined,
          title: 'Add your names and ID number',
          message:
              'Your account has none yet. They must match your national ID and M-Pesa; staff check them before '
              'your first withdrawal.',
          actionLabel: 'Add them',
          onAction: () => context.go('/account/details'),
        ),
      if (account.value case final a?) ?_nextStep(context, a, wallet.value?.debtKes ?? 0),
      const _BalanceHero(),
      const _TodayCard(),
    ];
    final side = const <Widget>[OpenDrawsCard()];
    return Scaffold(
      appBar: AppBar(title: const BrandMark(height: 30)),
      body: SafeArea(
        top: false,
        child: RefreshIndicator(
          onRefresh: () async {
            ref
              ..invalidate(accountProvider)
              ..invalidate(walletStatementProvider)
              ..invalidate(todayProvider);
            ref.invalidate(drawsProvider);
            await ref.read(drawsProvider.future);
          },
          child: ListView(
            padding: const EdgeInsets.fromLTRB(Space.lg, Space.sm, Space.lg, Space.xxl),
            children: [
              ContentFrame(
                wide: true,
                child: wide
                    ? Row(
                        crossAxisAlignment: CrossAxisAlignment.start,
                        children: [
                          Expanded(child: Staggered(children: cards)),
                          const SizedBox(width: Space.xl),
                          Expanded(child: Staggered(children: side)),
                        ],
                      )
                    : Staggered(children: [...cards, ...side]),
              ),
            ],
          ),
        ),
      ),
    );
  }

  /// The one thing to do next, if there is one: confirm the number, wait out a pause, repay, or verify the ID that
  /// withdrawals will need (ADR 0022: identity is checked before the first withdrawal, not before a play).
  Widget? _nextStep(BuildContext context, Account a, int debtKes) {
    final paused = a.restriction;
    final (tone, title, body, action, route) = switch (a) {
      Account(status: 'suspended') => (
        StatusTone.error,
        'Your account is suspended',
        'Play, top-ups and withdrawals are paused. Contact support from your account.',
        null,
        null,
      ),
      Account(phoneVerified: false) => (
        StatusTone.pending,
        'Confirm your number',
        'Your first M-Pesa top-up proves the number is yours, and lets you play.',
        'Top up',
        '/wallet/top-up',
      ),
      _ when paused != null => (
        StatusTone.warning,
        paused.kind == 'self_exclusion' ? 'You are self-excluded' : 'You are taking a break',
        paused.until == null ? 'Play is paused.' : 'Play is paused until ${formatEat(paused.until!)}.',
        null,
        null,
      ),
      _ when debtKes > 0 => (
        StatusTone.warning,
        'Repay a reversed top-up',
        '${formatKes(debtKes)} is owed; your next top-up repays it first, then play and withdrawals resume.',
        'Top up',
        '/wallet/top-up',
      ),
      Account(kycStatus: KycStatus.none) => (
        StatusTone.neutral,
        'Verify your identity before you withdraw',
        'Winnings are paid out once staff have checked your national ID. You can play in the meantime.',
        'Verify my ID',
        '/account/verify',
      ),
      Account(kycStatus: KycStatus.pending || KycStatus.manualReview) => (
        StatusTone.pending,
        'Your ID is being checked',
        'Staff review each ID by hand; withdrawals open once it is approved.',
        null,
        null,
      ),
      Account(kycStatus: KycStatus.rejected) => (
        StatusTone.warning,
        'Your ID was not accepted',
        'Check the details and submit it again before you withdraw.',
        'Submit again',
        '/account/verify',
      ),
      _ => (StatusTone.neutral, null, null, null, null),
    };
    if (title == null) return null;
    return InlineBanner(
      tone: tone,
      title: title,
      message: body,
      actionLabel: action,
      onAction: route == null ? null : () => context.go(route),
    );
  }
}

/// The balance as a big light numeral over two soft glows, with frosted quick actions under it (redesign 2026-11-11).
class _BalanceHero extends ConsumerWidget {
  const _BalanceHero();

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final wallet = ref.watch(walletStatementProvider);
    final t = Theme.of(context).textTheme;
    final c = context.colors;
    return Reveal(
      child: GlowPanel(
        child: AsyncView(
          value: wallet,
          what: 'your balance',
          onRetry: () => ref.invalidate(walletStatementProvider),
          loading: const Skeleton(height: 64, width: 200),
          data: (w) => Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Row(
                children: [
                  Expanded(
                    child: Text('Confirmed balance', style: t.labelLarge?.copyWith(color: c.inkMuted)),
                  ),
                  RoundIconButton(
                    icon: Icons.chevron_right_rounded,
                    tooltip: 'Wallet',
                    dark: true,
                    onPressed: () => context.go('/wallet'),
                  ),
                ],
              ),
              // the balance beside a turning gold coin (3D, depth pass)
              Row(
                children: [
                  Expanded(
                    child: CountUp(
                      key: ValueKey(w.balanceKes),
                      value: w.balanceKes,
                      format: formatKes,
                      builder: (text) => BigAmount(text: text),
                    ),
                  ),
                  const SizedBox(width: Space.sm),
                  const Coin3D(size: 64),
                ],
              ),
              if (w.debtKes > 0) ...[
                const SizedBox(height: Space.sm),
                Text('Owed after a reversed top-up: ${formatKes(w.debtKes)}', style: t.bodySmall),
              ],
              const SizedBox(height: Space.lg),
              GlassChipRow(
                chips: [
                  (Icons.add_rounded, 'Top up', () => context.go('/wallet/top-up')),
                  (Icons.donut_large_rounded, 'Play', () => context.go('/play')),
                  (Icons.arrow_outward_rounded, 'Withdraw', () => context.go('/wallet/withdraw')),
                ],
              ),
            ],
          ),
        ),
      ),
    );
  }
}

/// A welcome by name (owner, 2026-10-30): "Karibu" is the welcome, the name the player gave at sign-up.
class _Greeting extends StatelessWidget {
  const _Greeting({required this.name});

  final String? name;

  @override
  Widget build(BuildContext context) {
    final t = Theme.of(context).textTheme;
    final c = context.colors;
    return Padding(
      padding: const EdgeInsets.only(left: Space.xs, top: Space.sm),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Semantics(header: true, child: Text(name == null ? 'Karibu' : 'Karibu, $name', style: t.headlineLarge)),
          const SizedBox(height: Space.xs),
          Text('Your wallet, today\'s plays and the open draws.', style: t.bodyMedium?.copyWith(color: c.inkMuted)),
        ],
      ),
    );
  }
}

class _TodayCard extends ConsumerWidget {
  const _TodayCard();

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final today = ref.watch(todayProvider);
    final t = Theme.of(context).textTheme;
    final c = context.colors;
    return AppCard(
      title: 'Today',
      icon: Icons.today_rounded,
      child: AsyncView(
        value: today,
        what: "today's plays",
        onRetry: () => ref.invalidate(todayProvider),
        data: (d) {
          final total = d.plays + d.playsLeft;
          final chances = '${d.chances} draw ${d.chances == 1 ? 'chance' : 'chances'}';
          return Column(
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              MergeSemantics(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Text(
                      d.plays == 0 ? 'No plays yet today' : '${d.plays} ${d.plays == 1 ? 'play' : 'plays'} today',
                      style: t.titleMedium,
                    ),
                    const SizedBox(height: Space.xxs),
                    Text(
                      d.plays == 0 ? '${d.playsLeft} of $total left · $chances' : '${d.playsLeft} left · $chances',
                      style: t.bodySmall,
                    ),
                    if (d.lucky) ...[
                      const SizedBox(height: Space.sm),
                      Row(
                        mainAxisSize: MainAxisSize.min,
                        children: [
                          ExcludeSemantics(child: Icon(Icons.star_rounded, size: 18, color: c.gold)),
                          const SizedBox(width: Space.xs),
                          Flexible(
                            child: Text('Lucky day · chances doubled', style: t.labelMedium?.copyWith(color: c.goldText)),
                          ),
                        ],
                      ),
                    ],
                  ],
                ),
              ),
              if (total > 0) ...[
                const SizedBox(height: Space.md),
                _PlaysChart(plays: d.tickets, done: d.plays, total: total),
              ],
              const SizedBox(height: Space.lg),
              FilledButton.icon(
                onPressed: () => context.go('/play'),
                icon: const Icon(Icons.donut_large_rounded),
                label: Text(d.playsLeft > 0 ? 'Go to Play · ${formatKes(d.priceKes)} a play' : 'See your plays'),
              ),
            ],
          );
        },
      ),
    );
  }
}

/// Today's plays as bars, one per play the day allows: a filled bar is a play made, a pale one a play left, the latest
/// play glows and a lucky one carries the star. It shows only what the server reported, nothing estimated; a day
/// allowing more plays than fit falls back to a ring.
class _PlaysChart extends StatelessWidget {
  const _PlaysChart({required this.plays, required this.done, required this.total});

  final List<Play> plays;
  final int done;
  final int total;

  static const _maxBars = 12;

  @override
  Widget build(BuildContext context) {
    final c = context.colors;
    final t = Theme.of(context).textTheme;
    final label = '$done of $total plays made today';
    if (total > _maxBars) {
      return Semantics(
        label: label,
        excludeSemantics: true,
        child: Align(
          alignment: Alignment.centerLeft,
          child: RingGauge(value: done / total, child: Text('$done/$total', style: t.titleSmall)),
        ),
      );
    }
    final lucky = {for (final p in plays) if (p.spin.lucky) p.seq};
    return GlowBars(
      semanticsLabel: label,
      bars: [
        for (var i = 1; i <= total; i++)
          GlowBar(
            value: i <= done ? 1 : 0,
            label: '$i',
            selected: i == done,
            mark: lucky.contains(i) ? Icon(Icons.star_rounded, size: 16, color: c.gold) : null,
          ),
      ],
    );
  }
}
