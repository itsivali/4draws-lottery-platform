import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../../app/providers.dart';
import '../../../app/theme/tokens.dart';
import '../../../core/widgets/brand_mark.dart';

class StartingScreen extends StatelessWidget {
  const StartingScreen({super.key});

  @override
  Widget build(BuildContext context) => Scaffold(
    body: Center(
      child: Semantics(
        label: 'Starting',
        liveRegion: true,
        child: const Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            BrandMark(height: 56),
            SizedBox(height: Space.xxl),
            SizedBox.square(dimension: 28, child: CircularProgressIndicator(strokeWidth: 3)),
          ],
        ),
      ),
    ),
  );
}

/// The saved session could not be renewed because the network is down. The session is kept: nothing is signed out.
class OfflineScreen extends ConsumerStatefulWidget {
  const OfflineScreen({super.key});

  @override
  ConsumerState<OfflineScreen> createState() => _OfflineScreenState();
}

class _OfflineScreenState extends ConsumerState<OfflineScreen> {
  bool _busy = false;

  Future<void> _retry() async {
    setState(() => _busy = true);
    await ref.read(sessionManagerProvider).restore();
    if (mounted) setState(() => _busy = false);
  }

  @override
  Widget build(BuildContext context) {
    final t = Theme.of(context).textTheme;
    final c = context.colors;
    return Scaffold(
      body: Center(
        child: Padding(
          padding: const EdgeInsets.all(Space.xl),
          child: ConstrainedBox(
            constraints: const BoxConstraints(maxWidth: 420),
            child: Column(
              mainAxisSize: MainAxisSize.min,
              children: [
                Container(
                  width: 72,
                  height: 72,
                  decoration: BoxDecoration(color: c.surfaceMuted, shape: BoxShape.circle),
                  child: ExcludeSemantics(child: Icon(Icons.cloud_off_rounded, size: 32, color: c.inkMuted)),
                ),
                const SizedBox(height: Space.xl),
                Text("You're offline", style: t.headlineSmall, textAlign: TextAlign.center),
                const SizedBox(height: Space.sm),
                Text(
                  "We couldn't reach the server to resume your session. Nothing has changed with your account; "
                  'connect and try again.',
                  style: t.bodyMedium?.copyWith(color: c.inkMuted),
                  textAlign: TextAlign.center,
                ),
                const SizedBox(height: Space.xl),
                FilledButton(onPressed: _busy ? null : _retry, child: Text(_busy ? 'Trying…' : 'Try again')),
              ],
            ),
          ),
        ),
      ),
    );
  }
}
