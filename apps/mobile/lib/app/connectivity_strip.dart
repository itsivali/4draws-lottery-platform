import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../features/account/application/account_providers.dart';
import '../features/draws/application/draws_providers.dart';
import '../features/play/application/play_providers.dart';
import '../features/wallet/application/wallet_providers.dart';
import 'providers.dart';
import 'theme/tokens.dart';

/// Re-reads what the screens show (the balance, the day's plays, the account, the draws): after the connection comes
/// back, and when the app returns from the background.
void refreshAll(WidgetRef ref) {
  ref
    ..invalidate(walletStatementProvider)
    ..invalidate(topUpsProvider)
    ..invalidate(payoutsProvider)
    ..invalidate(accountProvider)
    ..invalidate(todayProvider)
    ..invalidate(drawsProvider);
}

/// A slim strip across the top of every screen while the server cannot be reached (MASTER.md §11): what is shown may
/// be out of date, and "Retry" asks again. It disappears by itself the moment any request gets an answer.
class ConnectivityStrip extends ConsumerWidget {
  const ConnectivityStrip({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final reachability = ref.watch(reachabilityProvider);
    return ValueListenableBuilder<bool>(
      valueListenable: reachability,
      builder: (context, reached, _) {
        if (reached) return const SizedBox.shrink();
        final c = context.colors;
        final t = Theme.of(context).textTheme;
        return Semantics(
          liveRegion: true,
          child: Material(
            color: c.warning.withValues(alpha: 0.10),
            child: SafeArea(
              bottom: false,
              child: Padding(
                padding: const EdgeInsets.fromLTRB(Space.lg, Space.sm, Space.sm, Space.sm),
                child: Row(
                  children: [
                    ExcludeSemantics(child: Icon(Icons.cloud_off_rounded, size: 18, color: c.warning)),
                    const SizedBox(width: Space.sm),
                    Expanded(
                      child: Text(
                        "Can't reach the server. What you see may be out of date.",
                        style: t.labelMedium?.copyWith(color: c.ink),
                      ),
                    ),
                    TextButton(
                      style: TextButton.styleFrom(
                        foregroundColor: c.warning,
                        minimumSize: const Size(Breakpoints.minTouch, 36),
                      ),
                      onPressed: () => refreshAll(ref),
                      child: const Text('Retry'),
                    ),
                  ],
                ),
              ),
            ),
          ),
        );
      },
    );
  }
}
