import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../../app/theme/tokens.dart';
import '../../../core/api/api_failure.dart';
import '../../../core/api/idempotency.dart';
import '../../../core/formatting/dates.dart';
import '../../../core/formatting/money.dart';
import '../../../core/widgets/app_card.dart';
import '../../../core/widgets/async_view.dart';
import '../../../core/widgets/confirm_sheet.dart';
import '../../../core/widgets/content_frame.dart';
import '../../../core/widgets/failure_text.dart';
import '../../../core/widgets/status_pill.dart';
import '../application/account_providers.dart';
import '../domain/account_models.dart';

/// Deposit limits, breaks and self-exclusion. Every control shows the server's state and says so when an action
/// fails; nothing here tries to talk the player out of a limit or a break.
class ResponsibleGamingScreen extends ConsumerStatefulWidget {
  const ResponsibleGamingScreen({super.key});

  @override
  ConsumerState<ResponsibleGamingScreen> createState() => _ResponsibleGamingScreenState();
}

class _ResponsibleGamingScreenState extends ConsumerState<ResponsibleGamingScreen> {
  String? _busy;

  Future<void> _run(String what, Future<void> Function(String key) action, {required String done}) async {
    setState(() => _busy = what);
    final key = newIdempotencyKey();
    try {
      try {
        await action(key);
      } on ApiFailure catch (f) {
        if (!f.outcomeUncertain) rethrow;
        await action(key); // a lost answer: the same key is replayed, never applied twice
      }
      ref
        ..invalidate(accountProvider)
        ..invalidate(limitsProvider);
      if (mounted) ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(done)));
    } on ApiFailure catch (f) {
      if (mounted) {
        final d = describeFailure(f);
        ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text('Not saved: ${d.title}')));
      }
    } finally {
      if (mounted) setState(() => _busy = null);
    }
  }

  Future<void> _editLimit(String period, LimitState state) async {
    final controller = TextEditingController(text: state.current?.amountKes.toString() ?? '');
    final amount = await showDialog<int>(
      context: context,
      builder: (ctx) => AlertDialog(
        title: Text('${_periodLabel(period)} deposit limit'),
        content: Column(
          mainAxisSize: MainAxisSize.min,
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            TextField(
              controller: controller,
              autofocus: true,
              keyboardType: TextInputType.number,
              inputFormatters: [FilteringTextInputFormatter.digitsOnly],
              decoration: const InputDecoration(prefixText: 'KES ', labelText: 'Most you can deposit'),
            ),
            const SizedBox(height: Space.md),
            Text(
              'Lowering a limit applies now. Raising it applies after a cooling-off period.',
              style: Theme.of(ctx).textTheme.bodySmall,
            ),
          ],
        ),
        actions: [
          TextButton(onPressed: () => Navigator.pop(ctx), child: const Text('Cancel')),
          FilledButton(
            onPressed: () {
              final n = int.tryParse(controller.text);
              if (n != null && n > 0) Navigator.pop(ctx, n);
            },
            child: const Text('Save'),
          ),
        ],
      ),
    );
    controller.dispose();
    if (amount == null) return;
    await _run(
      'limit-$period',
      (key) => ref.read(accountRepositoryProvider).setLimit(period: period, amountKes: amount, idempotencyKey: key),
      done: 'Limit saved',
    );
  }

  Future<void> _timeout(String duration, String label) async {
    final ok = await confirmAction(
      context,
      title: 'Take a $label break?',
      facts: [('Break', label), ('During the break', 'No plays or top-ups. You can still withdraw.')],
      note: 'A break cannot be ended early.',
      confirmLabel: 'Start my $label break',
    );
    if (!ok) return;
    await _run(
      'timeout',
      (key) => ref.read(accountRepositoryProvider).startTimeout(duration: duration, idempotencyKey: key),
      done: 'Your break has started',
    );
  }

  Future<void> _exclude(String term, String label) async {
    final ok = await confirmAction(
      context,
      title: 'Self-exclude ${term == 'permanent' ? 'permanently' : 'for $label'}?',
      facts: [
        ('Self-exclusion', label),
        ('While excluded', 'No plays or top-ups. You can still withdraw your balance.'),
      ],
      note: 'Self-exclusion cannot be shortened or cancelled.',
      confirmLabel: 'Self-exclude',
      destructive: true,
    );
    if (!ok) return;
    await _run(
      'exclude',
      (key) => ref.read(accountRepositoryProvider).selfExclude(term: term, idempotencyKey: key),
      done: 'Self-exclusion has started',
    );
  }

  static String _periodLabel(String p) => switch (p) {
    'daily' => 'Daily',
    'weekly' => 'Weekly',
    'monthly' => 'Monthly',
    _ => p,
  };

  @override
  Widget build(BuildContext context) {
    final account = ref.watch(accountProvider);
    final limits = ref.watch(limitsProvider);
    final t = Theme.of(context).textTheme;
    final restricted = account.value?.restriction;
    return PageScaffold(
      title: 'Play safely',
      children: [
        if (restricted != null)
          AppCard(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                StatusPill(tone: StatusTone.pending, label: restricted.selfExclusion ? 'Self-excluded' : 'On a break'),
                const SizedBox(height: Space.sm),
                Text(
                  restricted.until == null
                      ? 'Permanently. Play and top-ups are closed.'
                      : 'Until ${formatEat(restricted.until!)}. Play and top-ups are closed until then.',
                  style: t.bodyMedium,
                ),
              ],
            ),
          ),
        AppCard(
          title: 'Deposit limits',
          child: AsyncView(
            value: limits,
            what: 'your limits',
            onRetry: () => ref.invalidate(limitsProvider),
            data: (map) => Column(
              children: [
                for (final p in const ['daily', 'weekly', 'monthly'])
                  ListTile(
                    contentPadding: EdgeInsets.zero,
                    title: Text(_periodLabel(p)),
                    subtitle: Text(
                      [
                        map[p]?.current == null ? 'No limit' : formatKes(map[p]!.current!.amountKes),
                        if (map[p]?.pending case final pending?)
                          'rising to ${formatKes(pending.amountKes)} from ${formatEat(pending.effectiveFrom)}',
                      ].join(' · '),
                    ),
                    trailing: _busy == 'limit-$p'
                        ? const SizedBox.square(dimension: 20, child: CircularProgressIndicator(strokeWidth: 2))
                        : TextButton(
                            onPressed: _busy == null ? () => _editLimit(p, map[p] ?? const LimitState()) : null,
                            child: const Text('Change'),
                          ),
                  ),
              ],
            ),
          ),
        ),
        AppCard(
          title: 'Take a break',
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Text('Pause play and top-ups for a while.', style: t.bodyMedium),
              const SizedBox(height: Space.md),
              Wrap(
                spacing: Space.sm,
                runSpacing: Space.sm,
                children: [
                  for (final (d, label) in const [('24h', '24-hour'), ('7d', '7-day'), ('30d', '30-day')])
                    OutlinedButton(onPressed: _busy == null ? () => _timeout(d, label) : null, child: Text(label)),
                ],
              ),
            ],
          ),
        ),
        AppCard(
          title: 'Self-exclusion',
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Text('Stop for a long time. It cannot be shortened once started.', style: t.bodyMedium),
              const SizedBox(height: Space.md),
              Wrap(
                spacing: Space.sm,
                runSpacing: Space.sm,
                children: [
                  for (final (term, label) in const [
                    ('6m', '6 months'),
                    ('1y', '1 year'),
                    ('permanent', 'Permanently'),
                  ])
                    OutlinedButton(onPressed: _busy == null ? () => _exclude(term, label) : null, child: Text(label)),
                ],
              ),
            ],
          ),
        ),
        AppCard(
          title: 'Getting help',
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Text(
                'If gambling stops being fun, take a break with the tools above and talk to someone you trust. '
                'Free, confidential support is available from:',
                style: t.bodyMedium,
              ),
              const SizedBox(height: Space.sm),
              Text('Responsible Gambling Kenya', style: t.titleMedium),
              const SelectableText('responsiblegambling.or.ke'),
            ],
          ),
        ),
      ],
    );
  }
}
