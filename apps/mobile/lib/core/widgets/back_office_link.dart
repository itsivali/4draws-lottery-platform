import 'package:flutter/foundation.dart';
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:url_launcher/url_launcher.dart';

import '../../app/providers.dart';
import '../../app/theme/tokens.dart';

/// The web app's way to the staff back office (owner, 2026-10-28): a quiet text link, shown on the web only and only
/// when the build names a back office (`BACKOFFICE_URL`). It opens in a new tab; the player app itself never signs
/// staff in.
class BackOfficeLink extends ConsumerWidget {
  const BackOfficeLink({super.key, this.web = kIsWeb, this.onHero = false});

  /// Whether this is the web app (a parameter so tests can show the link).
  final bool web;

  /// On a hero surface (the signed-out page): the hero's muted text colour instead of muted ink.
  final bool onHero;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final url = ref.watch(appConfigProvider).backOfficeUrl;
    if (!web || url == null) return const SizedBox.shrink();
    final c = context.colors;
    return Align(
      alignment: onHero ? Alignment.center : Alignment.centerLeft,
      child: TextButton.icon(
        style: TextButton.styleFrom(
          foregroundColor: onHero ? c.onHeroMuted : c.inkMuted,
          padding: const EdgeInsets.symmetric(horizontal: Space.sm),
          // on the signed-out page it is small print under the card, not a row of its own
          minimumSize: onHero ? const Size(Breakpoints.minTouch, 36) : null,
          tapTargetSize: onHero ? MaterialTapTargetSize.shrinkWrap : null,
        ),
        icon: const Icon(Icons.admin_panel_settings_outlined, size: 18),
        label: const Text('Staff? Open the back office'),
        onPressed: () => launchUrl(url, mode: LaunchMode.externalApplication, webOnlyWindowName: '_blank'),
      ),
    );
  }
}
