import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import '../../../app/providers.dart';
import '../../../app/theme/tokens.dart';
import '../../../core/formatting/dates.dart';
import '../../../core/formatting/phone.dart';
import '../../../core/platform/preferences.dart';
import '../../../core/widgets/app_card.dart';
import '../../../core/widgets/async_view.dart';
import '../../../core/widgets/back_office_link.dart';
import '../../../core/widgets/confirm_sheet.dart';
import '../../../core/widgets/content_frame.dart';
import '../../../core/widgets/list_row.dart';
import '../../../core/widgets/status_pill.dart';
import '../application/account_providers.dart';
import '../domain/account_models.dart';

(StatusTone, String) kycPill(KycStatus s) => switch (s) {
  KycStatus.none => (StatusTone.warning, 'ID not verified'),
  KycStatus.pending || KycStatus.manualReview => (StatusTone.pending, 'ID being checked'),
  KycStatus.verified => (StatusTone.success, 'ID verified'),
  KycStatus.rejected => (StatusTone.error, 'ID not accepted'),
};

class AccountScreen extends ConsumerWidget {
  const AccountScreen({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final account = ref.watch(accountProvider);
    final hapticsOn = ref.watch(hapticsEnabledProvider);
    final hapticsSupported = ref.watch(hapticsSupportedProvider);
    final t = Theme.of(context).textTheme;
    final c = context.colors;
    Widget link(IconData icon, String title, String route, {String? subtitle}) =>
        ListRow(icon: icon, title: title, subtitle: subtitle, chevron: true, onTap: () => context.go(route));
    return PageScaffold(
      title: 'Account',
      onRefresh: () async => ref.invalidate(accountProvider),
      children: [
        AppCard(
          child: AsyncView(
            value: account,
            what: 'your account',
            onRetry: () => ref.invalidate(accountProvider),
            data: (a) => Row(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Container(
                  width: 52,
                  height: 52,
                  decoration: BoxDecoration(color: c.primaryContainer, shape: BoxShape.circle),
                  child: ExcludeSemantics(child: Icon(Icons.person_rounded, color: c.onPrimaryContainer, size: 28)),
                ),
                const SizedBox(width: Space.lg),
                Expanded(
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      Text(formatMsisdn(a.msisdn), style: t.headlineSmall),
                      Text('Member since ${formatDay(a.createdAt)} ${a.createdAt.year}', style: t.bodySmall),
                      const SizedBox(height: Space.md),
                      Wrap(
                        spacing: Space.sm,
                        runSpacing: Space.sm,
                        children: [
                          StatusPill(tone: kycPill(a.kycStatus).$1, label: kycPill(a.kycStatus).$2),
                          a.phoneVerified
                              ? const StatusPill(tone: StatusTone.success, label: 'Number confirmed')
                              : const StatusPill(tone: StatusTone.warning, label: 'Number not confirmed yet'),
                          if (a.status == 'suspended')
                            const StatusPill(tone: StatusTone.error, label: 'Account suspended'),
                          if (a.status == 'closed') const StatusPill(tone: StatusTone.neutral, label: 'Account closed'),
                          if (a.restriction case final r?)
                            StatusPill(
                              tone: StatusTone.pending,
                              label: r.selfExclusion
                                  ? (r.until == null
                                        ? 'Self-excluded permanently'
                                        : 'Self-excluded until ${formatEat(r.until!)}')
                                  : (r.until == null ? 'On a break' : 'Break until ${formatEat(r.until!)}'),
                            ),
                          if (a.closureRequestedAt != null)
                            const StatusPill(tone: StatusTone.pending, label: 'Closure requested'),
                        ],
                      ),
                    ],
                  ),
                ),
              ],
            ),
          ),
        ),
        const SectionHeader(title: 'Your account'),
        RowGroup(
          children: [
            link(Icons.badge_rounded, 'Identity verification', '/account/verify'),
            link(Icons.lock_rounded, 'Change password', '/account/password'),
            link(
              Icons.alternate_email_rounded,
              'Email address',
              '/account/email',
              subtitle: account.value?.email ?? 'Add one so you can reset a forgotten password',
            ),
          ],
        ),
        const SectionHeader(title: 'Play safely'),
        RowGroup(
          children: [
            link(
              Icons.self_improvement_rounded,
              'Limits, breaks and self-exclusion',
              '/account/limits',
              subtitle: 'Set deposit limits, take a break or stop for a long time',
            ),
            link(
              Icons.menu_book_rounded,
              'How it works',
              '/account/how-it-works',
              subtitle: 'Plays, chances, draws and prizes',
            ),
            link(Icons.verified_rounded, 'Fairness', '/account/fairness', subtitle: 'Check your spins yourself'),
          ],
        ),
        const SectionHeader(title: 'Preferences'),
        RowGroup(
          children: [
            ListRow(
              icon: Icons.vibration_rounded,
              title: 'Wheel haptics',
              subtitle: hapticsSupported
                  ? 'Gentle vibrations as the wheel slows and stops. Your phone’s own settings still apply.'
                  : 'Not available in the browser.',
              trailing: Switch(
                value: hapticsSupported && hapticsOn,
                onChanged: hapticsSupported ? (v) => ref.read(hapticsEnabledProvider.notifier).set(v) : null,
              ),
            ),
          ],
        ),
        const SizedBox(height: Space.sm),
        Column(
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            OutlinedButton.icon(
              style: OutlinedButton.styleFrom(
                foregroundColor: c.error,
                side: BorderSide(color: c.error),
              ),
              icon: const Icon(Icons.logout_rounded),
              label: const Text('Sign out'),
              onPressed: () async {
                final ok = await confirmAction(
                  context,
                  title: 'Sign out?',
                  facts: const [],
                  confirmLabel: 'Sign out',
                  destructive: true,
                );
                if (ok) await ref.read(sessionManagerProvider).signOut();
              },
            ),
            const SizedBox(height: Space.sm),
            TextButton(
              style: TextButton.styleFrom(foregroundColor: c.inkMuted),
              onPressed: () => context.go('/account/close'),
              child: const Text('Close my account'),
            ),
            const SizedBox(height: Space.sm),
            const BackOfficeLink(),
          ],
        ),
      ],
    );
  }
}
