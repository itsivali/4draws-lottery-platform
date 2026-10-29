import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import '../../../app/theme/tokens.dart';
import '../../../core/formatting/dates.dart';
import '../../../core/formatting/money.dart';
import '../../../core/widgets/app_card.dart';
import '../../../core/widgets/async_view.dart';
import '../../../core/widgets/brand_mark.dart';
import '../../../core/widgets/content_frame.dart';
import '../../../core/widgets/hero_card.dart';
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
import '../../wallet/application/wallet_providers.dart';

class HomeScreen extends ConsumerWidget {
  const HomeScreen({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final wide = widthClassOf(context) == WidthClass.expanded;
    final account = ref.watch(accountProvider);
    final wallet = ref.watch(walletStatementProvider);
    final cards = <Widget>[
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

class _BalanceHero extends ConsumerWidget {
  const _BalanceHero();

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final wallet = ref.watch(walletStatementProvider);
    final t = Theme.of(context).textTheme;
    final c = context.colors;
    final heroButton = OutlinedButton.styleFrom(
      foregroundColor: c.onHero,
      side: BorderSide(color: c.onHero.withValues(alpha: 0.45)),
      minimumSize: const Size(Breakpoints.minTouch, 44),
    );
    return HeroCard(
      child: AsyncView(
        value: wallet,
        what: 'your balance',
        onRetry: () => ref.invalidate(walletStatementProvider),
        loading: const Skeleton(height: 48, width: 180),
        data: (w) => Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Text('Confirmed balance', style: t.labelMedium?.copyWith(color: c.onHeroMuted)),
            const SizedBox(height: Space.xs),
            CountUp(
              key: ValueKey(w.balanceKes),
              value: w.balanceKes,
              format: formatKes,
              style: t.displayMedium?.copyWith(color: c.onHero),
            ),
            if (w.debtKes > 0) ...[
              const SizedBox(height: Space.sm),
              Text(
                'Owed after a reversed top-up: ${formatKes(w.debtKes)}',
                style: t.bodySmall?.copyWith(color: c.onHeroMuted),
              ),
            ],
            const SizedBox(height: Space.lg),
            Wrap(
              spacing: Space.sm,
              runSpacing: Space.sm,
              children: [
                OutlinedButton.icon(
                  style: heroButton,
                  onPressed: () => context.go('/wallet/top-up'),
                  icon: const Icon(Icons.add_rounded, size: 20),
                  label: const Text('Top up'),
                ),
                OutlinedButton.icon(
                  style: heroButton,
                  onPressed: () => context.go('/wallet'),
                  icon: const Icon(Icons.account_balance_wallet_rounded, size: 20),
                  label: const Text('Wallet'),
                ),
              ],
            ),
          ],
        ),
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
              Row(
                children: [
                  _PlaysRing(done: d.plays, total: total),
                  const SizedBox(width: Space.lg),
                  Expanded(
                    child: MergeSemantics(
                      child: Column(
                        crossAxisAlignment: CrossAxisAlignment.start,
                        children: [
                          Text(
                            d.plays == 0 ? 'No plays yet today' : '${d.plays} ${d.plays == 1 ? 'play' : 'plays'} today',
                            style: t.titleMedium,
                          ),
                          const SizedBox(height: Space.xxs),
                          Text(
                            d.plays == 0
                                ? '${d.playsLeft} of $total left · $chances'
                                : '${d.playsLeft} left · $chances',
                            style: t.bodySmall,
                          ),
                          if (d.lucky) ...[
                            const SizedBox(height: Space.sm),
                            Row(
                              mainAxisSize: MainAxisSize.min,
                              children: [
                                ExcludeSemantics(child: Icon(Icons.star_rounded, size: 18, color: c.gold)),
                                const SizedBox(width: Space.xs),
                                Text('Lucky day · chances doubled', style: t.labelMedium?.copyWith(color: c.goldText)),
                              ],
                            ),
                          ],
                        ],
                      ),
                    ),
                  ),
                ],
              ),
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

/// Plays done today as a ring: a quiet progress mark, not a score.
class _PlaysRing extends StatelessWidget {
  const _PlaysRing({required this.done, required this.total});

  final int done;
  final int total;

  @override
  Widget build(BuildContext context) {
    final c = context.colors;
    final t = Theme.of(context).textTheme;
    return ExcludeSemantics(
      child: SizedBox.square(
        dimension: 64,
        child: Stack(
          alignment: Alignment.center,
          children: [
            TweenAnimationBuilder<double>(
              tween: Tween(end: total == 0 ? 0 : done / total),
              duration: context.reduceMotion ? Duration.zero : Motion.emphasis,
              curve: Motion.settle,
              builder: (_, v, _) => SizedBox.expand(
                child: CircularProgressIndicator(
                  value: v,
                  strokeWidth: 6,
                  strokeCap: StrokeCap.round,
                  color: c.primary,
                  backgroundColor: c.surfaceMuted,
                ),
              ),
            ),
            Text('$done/$total', style: t.titleSmall),
          ],
        ),
      ),
    );
  }
}
