import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import '../../../app/theme/tokens.dart';
import '../../../core/formatting/dates.dart';
import '../../../core/widgets/async_view.dart';
import '../../../core/widgets/content_frame.dart';
import '../../../core/widgets/list_row.dart';
import '../application/account_providers.dart';

/// How the account can be signed in to (owner, 2026-10-31): a password, and the Google or Apple identities linked
/// to it (ADR 0020). Linking happens from the sign-in screen, not here.
class SignInsScreen extends ConsumerWidget {
  const SignInsScreen({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final signIns = ref.watch(signInsProvider);
    final t = Theme.of(context).textTheme;
    final c = context.colors;
    return PageScaffold(
      title: 'Sign-in methods',
      onRefresh: () async => ref.invalidate(signInsProvider),
      children: [
        AsyncView(
          value: signIns,
          what: 'your sign-in methods',
          onRetry: () => ref.invalidate(signInsProvider),
          data: (m) {
            final google = m.linked('google');
            final apple = m.linked('apple');
            return Column(
              crossAxisAlignment: CrossAxisAlignment.stretch,
              children: [
                RowGroup(
                  children: [
                    ListRow(
                      icon: Icons.lock_rounded,
                      title: 'Password',
                      subtitle: m.password ? 'Set' : 'Not set: you sign in with Google or Apple. Tap to set one',
                      chevron: true,
                      onTap: () => context.go(m.password ? '/account/password' : '/account/password?set=1'),
                    ),
                    ListRow(
                      icon: Icons.g_mobiledata_rounded,
                      title: 'Google',
                      subtitle: google == null ? 'Not linked' : 'Linked ${formatEat(google.linkedAt)}',
                    ),
                    ListRow(
                      icon: Icons.apple_rounded,
                      title: 'Apple',
                      subtitle: apple == null ? 'Not linked' : 'Linked ${formatEat(apple.linkedAt)}',
                    ),
                  ],
                ),
                const SizedBox(height: Space.lg),
                Padding(
                  padding: const EdgeInsets.symmetric(horizontal: Space.xs),
                  child: Text(
                    'Google and Apple are linked from the sign-in screen, with "Continue with Google" or "Continue '
                    'with Apple", not from here. A linked identity signs in to this account with no password.',
                    style: t.bodySmall?.copyWith(color: c.inkMuted),
                  ),
                ),
              ],
            );
          },
        ),
      ],
    );
  }
}
