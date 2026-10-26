import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../../app/theme/tokens.dart';
import '../../../core/api/api_failure.dart';
import '../../../core/api/idempotency.dart';
import '../../../core/formatting/dates.dart';
import '../../../core/widgets/app_card.dart';
import '../../../core/widgets/async_view.dart';
import '../../../core/widgets/busy_button.dart';
import '../../../core/widgets/confirm_sheet.dart';
import '../../../core/widgets/content_frame.dart';
import '../../../core/widgets/failure_text.dart';
import '../../../core/widgets/status_pill.dart';
import '../application/account_providers.dart';

/// Asks for the account to be closed. Closing itself is done by staff once any balance is paid out; the screen says
/// exactly that and never claims the account is already deleted.
class ClosureScreen extends ConsumerStatefulWidget {
  const ClosureScreen({super.key});

  @override
  ConsumerState<ClosureScreen> createState() => _ClosureScreenState();
}

class _ClosureScreenState extends ConsumerState<ClosureScreen> {
  var _busy = false;
  String? _error;

  Future<void> _request() async {
    final ok = await confirmAction(
      context,
      title: 'Ask to close your account?',
      facts: const [('What happens', 'Staff close it after any balance is paid out to your M-Pesa number.')],
      note:
          'Records the law requires (payments, plays, tax) are kept for the period it sets. To stop playing straight '
          'away, use a break or self-exclusion.',
      confirmLabel: 'Ask to close my account',
      destructive: true,
    );
    if (!ok) return;
    setState(() {
      _busy = true;
      _error = null;
    });
    final key = newIdempotencyKey();
    try {
      try {
        await ref.read(accountRepositoryProvider).requestClosure(idempotencyKey: key);
      } on ApiFailure catch (f) {
        if (!f.outcomeUncertain) rethrow;
        await ref.read(accountRepositoryProvider).requestClosure(idempotencyKey: key);
      }
      ref.invalidate(accountProvider);
    } on ApiFailure catch (f) {
      setState(() => _error = describeFailure(f).title);
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    final account = ref.watch(accountProvider);
    final t = Theme.of(context).textTheme;
    return PageScaffold(
      title: 'Close account',
      children: [
        AsyncView(
          value: account,
          what: 'your account',
          onRetry: () => ref.invalidate(accountProvider),
          data: (a) => AppCard(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.stretch,
              children: [
                if (a.closureRequestedAt != null) ...[
                  const Align(
                    alignment: Alignment.centerLeft,
                    child: StatusPill(tone: StatusTone.pending, label: 'Closure requested'),
                  ),
                  const SizedBox(height: Space.md),
                  Text(
                    'You asked on ${formatEat(a.closureRequestedAt!)}. Staff will pay out any balance and close the account. '
                    'Until then you can still sign in and withdraw.',
                    style: t.bodyMedium,
                  ),
                ] else ...[
                  Text(
                    'You can ask for your account to be closed. Staff close it once any balance has been paid out to your '
                    'M-Pesa number. Records the law requires are kept; everything else about you is no longer used.',
                    style: t.bodyMedium,
                  ),
                  if (_error != null) ...[
                    const SizedBox(height: Space.md),
                    Text(_error!, style: t.bodyMedium?.copyWith(color: context.colors.error)),
                  ],
                  const SizedBox(height: Space.xl),
                  BusyButton(label: 'Ask to close my account', busy: _busy, onPressed: _request),
                ],
              ],
            ),
          ),
        ),
      ],
    );
  }
}
