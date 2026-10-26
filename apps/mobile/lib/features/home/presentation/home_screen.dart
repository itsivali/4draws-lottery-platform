import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import '../../../app/theme/tokens.dart';
import '../../../core/formatting/money.dart';
import '../../../core/widgets/app_card.dart';
import '../../../core/widgets/async_view.dart';
import '../../../core/widgets/brand_mark.dart';
import '../../../core/widgets/content_frame.dart';
import '../../../core/widgets/skeleton.dart';
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
    final cards = <Widget>[
      if (account.value case final a?) ?_nextStep(context, a),
      const _BalanceCard(),
      const _TodayCard(),
    ];
    final side = const <Widget>[OpenDrawsCard()];
    return Scaffold(
      appBar: AppBar(title: const BrandMark(height: 26)),
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
                          Expanded(
                            child: Column(crossAxisAlignment: CrossAxisAlignment.stretch, children: _gap(cards)),
                          ),
                          const SizedBox(width: Space.xl),
                          Expanded(
                            child: Column(crossAxisAlignment: CrossAxisAlignment.stretch, children: _gap(side)),
                          ),
                        ],
                      )
                    : Column(crossAxisAlignment: CrossAxisAlignment.stretch, children: _gap([...cards, ...side])),
              ),
            ],
          ),
        ),
      ),
    );
  }

  static List<Widget> _gap(List<Widget> w) => [
    for (var i = 0; i < w.length; i++) ...[if (i > 0) const SizedBox(height: Space.lg), w[i]],
  ];

  Widget? _nextStep(BuildContext context, Account a) {
    final (title, body, action, route) = switch (a) {
      Account(kycStatus: KycStatus.none) => (
        'Verify your identity',
        'Needed before your first top-up and play.',
        'Verify my ID',
        '/account/verify',
      ),
      Account(kycStatus: KycStatus.pending || KycStatus.manualReview) => (
        'Your ID is being checked',
        'Staff review each ID by hand.',
        null,
        null,
      ),
      Account(phoneVerified: false, kycStatus: KycStatus.verified) => (
        'Confirm your number',
        'Your first M-Pesa top-up proves the number is yours.',
        'Top up',
        '/wallet/top-up',
      ),
      _ => (null, null, null, null),
    };
    if (title == null) return null;
    return AppCard(
      title: title,
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Text(body!, style: Theme.of(context).textTheme.bodyMedium),
          if (route != null) ...[
            const SizedBox(height: Space.md),
            FilledButton(onPressed: () => context.go(route), child: Text(action!)),
          ],
        ],
      ),
    );
  }
}

class _BalanceCard extends ConsumerWidget {
  const _BalanceCard();

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final wallet = ref.watch(walletStatementProvider);
    final t = Theme.of(context).textTheme;
    return AppCard(
      title: 'Wallet',
      trailing: TextButton(onPressed: () => context.go('/wallet'), child: const Text('Open')),
      child: AsyncView(
        value: wallet,
        what: 'your balance',
        onRetry: () => ref.invalidate(walletStatementProvider),
        loading: const Skeleton(height: 48, width: 180),
        data: (w) => Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Text('Confirmed balance', style: t.bodySmall),
            Text(formatKes(w.balanceKes), style: t.displaySmall),
            if (w.debtKes > 0)
              Text(
                'Owed after a reversed top-up: ${formatKes(w.debtKes)}',
                style: t.bodySmall?.copyWith(color: context.colors.warning),
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
    return AppCard(
      title: 'Today',
      child: AsyncView(
        value: today,
        what: "today's plays",
        onRetry: () => ref.invalidate(todayProvider),
        data: (d) => Column(
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            Text(
              d.plays == 0
                  ? 'No plays yet today. ${d.playsLeft} of 4 left.'
                  : '${d.plays} ${d.plays == 1 ? 'play' : 'plays'} today · ${d.playsLeft} left · '
                        '${d.chances} draw ${d.chances == 1 ? 'chance' : 'chances'}${d.lucky ? ' (lucky day, doubled)' : ''}',
              style: t.bodyLarge,
            ),
            const SizedBox(height: Space.md),
            FilledButton.icon(
              onPressed: () => context.go('/play'),
              icon: const Icon(Icons.donut_large_rounded),
              label: Text(d.playsLeft > 0 ? 'Go to Play · ${formatKes(d.priceKes)} a play' : 'See your plays'),
            ),
          ],
        ),
      ),
    );
  }
}
