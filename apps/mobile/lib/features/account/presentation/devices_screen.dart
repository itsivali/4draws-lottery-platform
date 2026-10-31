import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../../app/theme/tokens.dart';
import '../../../core/api/api_failure.dart';
import '../../../core/api/idempotency.dart';
import '../../../core/formatting/dates.dart';
import '../../../core/widgets/async_view.dart';
import '../../../core/widgets/confirm_sheet.dart';
import '../../../core/widgets/content_frame.dart';
import '../../../core/widgets/failure_text.dart';
import '../../../core/widgets/inline_banner.dart';
import '../../../core/widgets/list_row.dart';
import '../../../core/widgets/status_pill.dart';
import '../application/account_providers.dart';
import '../domain/account_models.dart';

/// The devices signed in to the account (owner, 2026-10-31), and one button to sign the others out.
class DevicesScreen extends ConsumerStatefulWidget {
  const DevicesScreen({super.key});

  @override
  ConsumerState<DevicesScreen> createState() => _DevicesScreenState();
}

class _DevicesScreenState extends ConsumerState<DevicesScreen> {
  var _busy = false;
  String? _error;

  Future<void> _signOutOthers(int others) async {
    final ok = await confirmAction(
      context,
      title: 'Sign out other devices?',
      facts: [('Devices', others == 1 ? '1 other device' : '$others other devices')],
      note: 'They will have to sign in again. This device stays signed in.',
      confirmLabel: 'Sign them out',
      destructive: true,
    );
    if (!ok || !mounted) return;
    setState(() {
      _busy = true;
      _error = null;
    });
    try {
      await ref.read(accountRepositoryProvider).signOutOtherDevices(idempotencyKey: newIdempotencyKey());
      ref.invalidate(sessionsProvider);
      await ref.read(sessionsProvider.future);
      if (!mounted) return;
      ScaffoldMessenger.of(context).showSnackBar(const SnackBar(content: Text('Other devices are signed out.')));
    } on ApiFailure catch (f) {
      if (!mounted) return;
      final d = describeFailure(f);
      setState(() => _error = [d.title, ?d.detail].join(': '));
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    final sessions = ref.watch(sessionsProvider);
    final t = Theme.of(context).textTheme;
    final c = context.colors;
    return PageScaffold(
      title: 'Devices signed in',
      onRefresh: () async => ref.invalidate(sessionsProvider),
      children: [
        Padding(
          padding: const EdgeInsets.only(left: Space.xs, bottom: Space.lg),
          child: Text(
            'Every phone and browser with this account open. If one is not yours, sign the others out and change '
            'your password.',
            style: t.bodyMedium?.copyWith(color: c.inkMuted),
          ),
        ),
        AsyncView(
          value: sessions,
          what: 'your devices',
          onRetry: () => ref.invalidate(sessionsProvider),
          data: (list) {
            final others = list.where((s) => !s.current).length;
            return Column(
              crossAxisAlignment: CrossAxisAlignment.stretch,
              children: [
                RowGroup(children: [for (final s in list) _SessionRow(session: s)]),
                if (_error != null) ...[
                  const SizedBox(height: Space.lg),
                  InlineBanner(tone: StatusTone.error, live: true, title: _error!),
                ],
                const SizedBox(height: Space.xl),
                OutlinedButton.icon(
                  style: OutlinedButton.styleFrom(
                    foregroundColor: c.error,
                    side: BorderSide(color: c.error),
                  ),
                  icon: const Icon(Icons.logout_rounded),
                  label: Text(_busy ? 'Signing out…' : 'Sign out other devices'),
                  onPressed: others == 0 || _busy ? null : () => _signOutOthers(others),
                ),
                if (others == 0) ...[
                  const SizedBox(height: Space.sm),
                  Text('Only this device is signed in.', style: t.bodySmall, textAlign: TextAlign.center),
                ],
              ],
            );
          },
        ),
      ],
    );
  }
}

class _SessionRow extends StatelessWidget {
  const _SessionRow({required this.session});

  final DeviceSession session;

  @override
  Widget build(BuildContext context) {
    final (icon, label) = switch (session.platform) {
      'android' => (Icons.phone_android_rounded, 'Android phone'),
      'ios' => (Icons.phone_iphone_rounded, 'iPhone or iPad'),
      _ => (Icons.language_rounded, 'Web browser'),
    };
    return ListRow(
      icon: icon,
      title: label,
      subtitle: 'Signed in ${formatEat(session.createdAt)}\nLast used ${formatEat(session.lastUsedAt)}',
      trailing: session.current
          ? const StatusPill(tone: StatusTone.success, label: 'This device', compact: true)
          : session.deviceVerdict == 'failed'
          ? const StatusPill(tone: StatusTone.warning, label: 'Unverified', compact: true)
          : null,
    );
  }
}
