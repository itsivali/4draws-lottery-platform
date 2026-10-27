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
import '../../../core/widgets/inline_banner.dart';
import '../../../core/widgets/list_row.dart';
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
        ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(
            content: Text(
              f.outcomeUncertain
                  ? 'Not confirmed: ${d.title}. Pull down to see whether it was saved.'
                  : 'Not saved: ${d.title}',
            ),
          ),
        );
      }
    } finally {
      if (mounted) setState(() => _busy = null);
    }
  }

  Future<void> _editLimit(String period, LimitState state) async {
    final amount = await showDialog<int>(
      context: context,
      builder: (ctx) => _LimitDialog(period: _periodLabel(period), initial: state.current?.amountKes),
    );
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
    final c = context.colors;
    final restricted = account.value?.restriction;
    return PageScaffold(
      title: 'Play safely',
      onRefresh: () async {
        ref
          ..invalidate(accountProvider)
          ..invalidate(limitsProvider);
      },
      children: [
        if (restricted != null)
          InlineBanner(
            tone: StatusTone.pending,
            title: restricted.selfExclusion ? 'Self-excluded' : 'On a break',
            message: restricted.until == null
                ? 'Permanently. Play and top-ups are closed.'
                : 'Until ${formatEat(restricted.until!)}. Play and top-ups are closed until then.',
          ),
        AppCard(
          title: 'Deposit limits',
          padding: const EdgeInsets.only(top: Space.lg),
          child: AsyncView(
            value: limits,
            what: 'your limits',
            onRetry: () => ref.invalidate(limitsProvider),
            data: (map) => Column(
              crossAxisAlignment: CrossAxisAlignment.stretch,
              children: [
                for (final (i, p) in const ['daily', 'weekly', 'monthly'].indexed) ...[
                  if (i > 0) Divider(indent: Space.lg, endIndent: Space.lg, color: c.border),
                  ListRow(
                    icon: Icons.speed_rounded,
                    title: _periodLabel(p),
                    subtitle: [
                      map[p]?.current == null ? 'No limit' : formatKes(map[p]!.current!.amountKes),
                      if (map[p]?.pending case final pending?)
                        'rising to ${formatKes(pending.amountKes)} from ${formatEat(pending.effectiveFrom)}',
                    ].join(' · '),
                    trailing: _busy == 'limit-$p'
                        ? const SizedBox.square(dimension: 20, child: CircularProgressIndicator(strokeWidth: 2))
                        : TextButton(
                            onPressed: _busy == null ? () => _editLimit(p, map[p] ?? const LimitState()) : null,
                            child: const Text('Change'),
                          ),
                  ),
                ],
              ],
            ),
          ),
        ),
        AppCard(
          title: 'Take a break',
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Text('Pause play and top-ups for a while.', style: t.bodyMedium?.copyWith(color: c.inkMuted)),
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
              Text(
                'Stop for a long time. It cannot be shortened once started.',
                style: t.bodyMedium?.copyWith(color: c.inkMuted),
              ),
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
          tinted: true,
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Text(
                'If gambling stops being fun, take a break with the tools above and talk to someone you trust. '
                'Free, confidential support is available from:',
                style: t.bodyMedium?.copyWith(color: c.onPrimaryContainer),
              ),
              const SizedBox(height: Space.md),
              Text('Responsible Gambling Kenya', style: t.titleMedium?.copyWith(color: c.onPrimaryContainer)),
              SelectableText('responsiblegambling.or.ke', style: t.bodyMedium?.copyWith(color: c.onPrimaryContainer)),
            ],
          ),
        ),
      ],
    );
  }
}

/// Asks for one limit. The controller lives with the dialog, so it outlives the exit animation.
class _LimitDialog extends StatefulWidget {
  const _LimitDialog({required this.period, required this.initial});

  final String period;
  final int? initial;

  @override
  State<_LimitDialog> createState() => _LimitDialogState();
}

class _LimitDialogState extends State<_LimitDialog> {
  late final _controller = TextEditingController(text: widget.initial?.toString() ?? '');
  String? _error;

  @override
  void dispose() {
    _controller.dispose();
    super.dispose();
  }

  void _save() {
    final n = int.tryParse(_controller.text);
    if (n == null || n <= 0) {
      setState(() => _error = 'Enter a whole number of shillings above zero.');
      return;
    }
    Navigator.pop(context, n);
  }

  @override
  Widget build(BuildContext context) => AlertDialog(
    title: Text('${widget.period} deposit limit'),
    content: Column(
      mainAxisSize: MainAxisSize.min,
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        TextField(
          controller: _controller,
          autofocus: true,
          keyboardType: TextInputType.number,
          inputFormatters: [FilteringTextInputFormatter.digitsOnly],
          onSubmitted: (_) => _save(),
          decoration: InputDecoration(prefixText: 'KES ', labelText: 'Most you can deposit', errorText: _error),
        ),
        const SizedBox(height: Space.md),
        Text(
          'Lowering a limit applies now. Raising it applies after a cooling-off period.',
          style: Theme.of(context).textTheme.bodySmall,
        ),
      ],
    ),
    actions: [
      TextButton(onPressed: () => Navigator.pop(context), child: const Text('Cancel')),
      FilledButton(onPressed: _save, child: const Text('Save')),
    ],
  );
}
