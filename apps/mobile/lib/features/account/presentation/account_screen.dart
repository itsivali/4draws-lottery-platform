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
import '../../../core/widgets/policy_pages.dart';
import '../../../core/widgets/status_pill.dart';
import '../application/account_providers.dart';
import '../domain/account_models.dart';

(StatusTone, String) kycPill(KycStatus s) => switch (s) {
  KycStatus.none => (StatusTone.warning, 'ID not verified'),
  KycStatus.pending || KycStatus.manualReview => (StatusTone.pending, 'ID being checked'),
  KycStatus.verified => (StatusTone.success, 'ID verified'),
  KycStatus.rejected => (StatusTone.error, 'ID not accepted'),
};

/// The player's profile (owner, 2026-10-31): who they are, then everything they manage about the account.
class AccountScreen extends ConsumerWidget {
  const AccountScreen({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final account = ref.watch(accountProvider);
    final hapticsOn = ref.watch(hapticsEnabledProvider);
    final hapticsSupported = ref.watch(hapticsSupportedProvider);
    final c = context.colors;
    final a = account.value;
    Widget link(IconData icon, String title, String route, {String? subtitle}) =>
        ListRow(icon: icon, title: title, subtitle: subtitle, chevron: true, onTap: () => context.go(route));
    final detailsSubtitle = switch (a) {
      null => null,
      Account(needsIdentity: true) => 'Add your first name, second name and ID number',
      Account(identity: AccountIdentity(status: IdentityStatus.verified)) => 'Names and ID number, verified',
      Account(identity: AccountIdentity(status: IdentityStatus.submitted)) => 'Names and ID number, being checked',
      _ => 'Names and ID number, checked before your first withdrawal',
    };
    return PageScaffold(
      title: 'Account',
      onRefresh: () async => ref.invalidate(accountProvider),
      children: [
        AppCard(
          child: AsyncView(
            value: account,
            what: 'your account',
            onRetry: () => ref.invalidate(accountProvider),
            data: (a) => _ProfileHeader(account: a),
          ),
        ),
        const SectionHeader(title: 'Your details'),
        RowGroup(
          children: [
            link(
              a?.needsIdentity ?? false ? Icons.person_add_alt_1_rounded : Icons.badge_rounded,
              'Your details',
              '/account/details',
              subtitle: detailsSubtitle,
            ),
            link(
              Icons.verified_user_rounded,
              'Identity verification',
              '/account/verify',
              subtitle: a == null ? null : kycPill(a.kycStatus).$2,
            ),
          ],
        ),
        const SectionHeader(title: 'Sign-in and security'),
        RowGroup(
          children: [
            link(Icons.lock_rounded, 'Change password', '/account/password'),
            link(
              Icons.alternate_email_rounded,
              'Email address',
              '/account/email',
              // the address itself is in the header above
              subtitle: a?.email == null
                  ? 'Add one so you can reset a forgotten password'
                  : (a!.emailVerified ? 'Where a reset link goes' : 'Not confirmed yet: open the link we emailed you'),
            ),
            link(
              Icons.devices_rounded,
              'Devices signed in',
              '/account/devices',
              subtitle: 'See every device with this account open; sign the others out',
            ),
            link(Icons.login_rounded, 'Sign-in methods', '/account/sign-ins', subtitle: 'Password and Google'),
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
        const SectionHeader(title: 'About'),
        const RowGroup(
          children: [
            PolicyRow(PolicyPage.privacy),
            PolicyRow(PolicyPage.terms),
            PolicyRow(PolicyPage.responsibleGambling),
            PolicyRow(PolicyPage.deleteAccount),
          ],
        ),
        const SectionHeader(title: 'Your data'),
        RowGroup(
          children: [
            link(
              Icons.download_rounded,
              'Get a copy of your data',
              '/account/data',
              subtitle: 'A report you can read, emailed to you',
            ),
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
        const SectionHeader(title: 'Account'),
        Column(
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            // the brand's indigo, as every other outlined button: signing out loses nothing, so it is not red (owner,
            // 2026-11-12; red is kept for what cannot be undone)
            OutlinedButton.icon(
              icon: const Icon(Icons.logout_rounded),
              label: const Text('Sign out'),
              onPressed: () async {
                final ok = await confirmAction(context, title: 'Sign out?', facts: const [], confirmLabel: 'Sign out');
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

/// Initials avatar, name (or the invitation to add it), number, email, member since, and the account's states.
class _ProfileHeader extends StatelessWidget {
  const _ProfileHeader({required this.account});

  final Account account;

  @override
  Widget build(BuildContext context) {
    final a = account;
    final t = Theme.of(context).textTheme;
    final c = context.colors;
    final initials = a.initials;
    return Row(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Container(
          width: 56,
          height: 56,
          alignment: Alignment.center,
          clipBehavior: Clip.antiAlias,
          decoration: BoxDecoration(color: c.primaryContainer, shape: BoxShape.circle),
          child: ExcludeSemantics(
            child: Stack(
              alignment: Alignment.center,
              fit: StackFit.expand,
              children: [
                Center(
                  child: initials == null
                      ? Icon(Icons.person_rounded, color: c.onPrimaryContainer, size: 28)
                      : Text(
                          initials,
                          style: t.titleLarge?.copyWith(color: c.onPrimaryContainer, fontWeight: FontWeight.w700),
                        ),
                ),
                // the Google account's photo over the initials, which stay when it cannot load (ADR 0035)
                if (a.photoUrl case final url?)
                  Image.network(
                    url,
                    key: const Key('account-photo'),
                    fit: BoxFit.cover,
                    errorBuilder: (_, _, _) => const SizedBox.shrink(),
                  ),
              ],
            ),
          ),
        ),
        const SizedBox(width: Space.lg),
        Expanded(
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              if (a.displayName case final name?)
                Text(name, style: t.headlineSmall)
              else if (a.needsIdentity)
                // the invitation stands where the name will: tapping it opens the form
                InkWell(
                  onTap: () => context.go('/account/details'),
                  borderRadius: BorderRadius.circular(Radii.sm),
                  child: Padding(
                    padding: const EdgeInsets.symmetric(vertical: Space.xxs),
                    child: Row(
                      mainAxisSize: MainAxisSize.min,
                      children: [
                        Text('Add your names', style: t.headlineSmall?.copyWith(color: c.primary)),
                        const SizedBox(width: Space.xs),
                        ExcludeSemantics(child: Icon(Icons.edit_rounded, size: 20, color: c.primary)),
                      ],
                    ),
                  ),
                )
              else
                Text(formatMsisdn(a.msisdn), style: t.headlineSmall),
              if (a.displayName != null || a.needsIdentity) Text(formatMsisdn(a.msisdn), style: t.bodySmall),
              if (a.email case final email?) Text(email, style: t.bodySmall),
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
                  if (a.emailNeedsConfirming)
                    const StatusPill(tone: StatusTone.warning, label: 'Email not confirmed yet'),
                  if (a.status == 'suspended') const StatusPill(tone: StatusTone.error, label: 'Account suspended'),
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
    );
  }
}
