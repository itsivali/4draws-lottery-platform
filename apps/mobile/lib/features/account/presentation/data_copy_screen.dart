import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import '../../../app/theme/tokens.dart';
import '../../../core/api/api_failure.dart';
import '../../../core/api/idempotency.dart';
import '../../../core/widgets/app_card.dart';
import '../../../core/widgets/async_view.dart';
import '../../../core/widgets/busy_button.dart';
import '../../../core/widgets/content_frame.dart';
import '../../../core/widgets/failure_text.dart';
import '../../../core/widgets/inline_banner.dart';
import '../../../core/widgets/policy_pages.dart';
import '../../../core/widgets/status_pill.dart';
import '../application/account_providers.dart';

/// "Get a copy of my data" (ADR 0038): the server builds a readable report of what it holds about the player and emails it to
/// the address on the account, once a day at most. The screen says where it will go before sending, and where it went
/// (masked) after; it never shows the data itself.
class DataCopyScreen extends ConsumerStatefulWidget {
  const DataCopyScreen({super.key});

  @override
  ConsumerState<DataCopyScreen> createState() => _DataCopyScreenState();
}

class _DataCopyScreenState extends ConsumerState<DataCopyScreen> {
  var _busy = false;
  String? _sentTo;
  ({String title, String? detail})? _error;

  Future<void> _send() async {
    setState(() {
      _busy = true;
      _error = null;
    });
    try {
      // sent once: an unclear answer is not retried here, since a second request the same day is refused anyway
      final to = await ref.read(accountRepositoryProvider).requestDataCopy(idempotencyKey: newIdempotencyKey());
      if (mounted) setState(() => _sentTo = to);
    } on ApiFailure catch (f) {
      if (mounted) setState(() => _error = describeFailure(f));
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    final account = ref.watch(accountProvider);
    final t = Theme.of(context).textTheme;
    final c = context.colors;
    return PageScaffold(
      title: 'Your data',
      children: [
        AsyncView(
          value: account,
          what: 'your account',
          onRetry: () => ref.invalidate(accountProvider),
          data: (a) => AppCard(
            title: 'Get a copy of your data',
            icon: Icons.download_rounded,
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.stretch,
              children: [
                Text(
                  'We email you a report of what we hold about you: your account, top-ups, plays, draws, prizes and '
                  'withdrawals, your limits and breaks, and the devices you signed in with. Open the attachment and '
                  'it shows as a page in your browser, which you can print or save.',
                  style: t.bodyMedium?.copyWith(color: c.inkMuted),
                ),
                const SizedBox(height: Space.md),
                Text(
                  'For your safety it shows only the last three digits of your ID number, and never your '
                  'password. You can ask once a day.',
                  style: t.bodyMedium?.copyWith(color: c.inkMuted),
                ),
                Align(
                  alignment: Alignment.centerLeft,
                  child: TextButton(
                    onPressed: () => openPolicy(ref, PolicyPage.privacy),
                    child: const Text('Your rights over your data'),
                  ),
                ),
                const SizedBox(height: Space.md),
                if (_sentTo != null)
                  InlineBanner(
                    tone: StatusTone.success,
                    live: true,
                    title: 'Sent to $_sentTo',
                    message: 'It can take a few minutes to arrive. Check your junk folder too.',
                  )
                else if (a.email == null)
                  InlineBanner(
                    tone: StatusTone.warning,
                    title: 'Add an email address first',
                    message: 'Your copy is sent to the email address on your account, and it has none yet.',
                    actionLabel: 'Add an email address',
                    onAction: () => context.go('/account/email'),
                  )
                else ...[
                  FactRow(label: 'It will be sent to', value: a.email!),
                  if (_error != null) ...[
                    const SizedBox(height: Space.lg),
                    InlineBanner(tone: StatusTone.error, live: true, title: _error!.title, message: _error!.detail),
                  ],
                  const SizedBox(height: Space.xl),
                  BusyButton(label: 'Email me my data', busy: _busy, onPressed: _send),
                ],
              ],
            ),
          ),
        ),
      ],
    );
  }
}
