import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:url_launcher/url_launcher.dart';

import '../../app/providers.dart';
import 'list_row.dart';

/// The public policy pages (S8): plain web pages on the site, readable without the app, as the stores require.
enum PolicyPage {
  privacy('privacy', 'Privacy policy', 'What we keep about you, why, and your rights', Icons.privacy_tip_outlined),
  terms('terms', 'Terms of use', 'The rules of the service', Icons.description_outlined),
  responsibleGambling(
    'responsible-gambling',
    'Responsible gambling',
    'Staying in control, and where to get help',
    Icons.health_and_safety_outlined,
  ),
  deleteAccount(
    'delete-account',
    'Delete your account',
    'How, and what the law makes us keep',
    Icons.no_accounts_outlined,
  );

  const PolicyPage(this.slug, this.title, this.subtitle, this.icon);

  final String slug;
  final String title;
  final String subtitle;
  final IconData icon;
}

/// Opens a web address in the browser (a new tab on the web); tests replace it.
final urlOpenerProvider = Provider<Future<bool> Function(Uri)>(
  (ref) =>
      (url) => launchUrl(url, mode: LaunchMode.externalApplication, webOnlyWindowName: '_blank'),
);

/// Opens a policy page in the browser.
Future<void> openPolicy(WidgetRef ref, PolicyPage page) =>
    ref.read(urlOpenerProvider)(ref.read(appConfigProvider).page(page.slug));

/// A row that opens a policy page.
class PolicyRow extends ConsumerWidget {
  const PolicyRow(this.page, {super.key});

  final PolicyPage page;

  @override
  Widget build(BuildContext context, WidgetRef ref) => ListRow(
    icon: page.icon,
    title: page.title,
    subtitle: page.subtitle,
    trailing: const Icon(Icons.open_in_new_rounded, size: 18),
    semanticsLabel: '${page.title}, opens in your browser',
    onTap: () => openPolicy(ref, page),
  );
}
