import 'package:flutter/material.dart';
import 'package:go_router/go_router.dart';

import '../core/widgets/brand_mark.dart';
import '../core/widgets/content_frame.dart';
import '../core/widgets/motion.dart';
import 'connectivity_strip.dart';
import 'theme/tokens.dart';

class _Destination {
  const _Destination(this.label, this.icon, this.selectedIcon);

  final String label;
  final IconData icon;
  final IconData selectedIcon;
}

// Material's convention: the outlined icon at rest, the filled one when chosen
const _destinations = [
  _Destination('Home', Icons.home_outlined, Icons.home_rounded),
  _Destination('Play', Icons.play_circle_outline_rounded, Icons.play_circle_rounded),
  _Destination('Draws', Icons.emoji_events_outlined, Icons.emoji_events_rounded),
  _Destination('Wallet', Icons.account_balance_wallet_outlined, Icons.account_balance_wallet_rounded),
  _Destination('Account', Icons.person_outline_rounded, Icons.person_rounded),
];

/// Five top-level destinations: Material's navigation bar on phones, a rail on tablets, an extended rail with the wordmark
/// on laptops and desktops (MASTER.md §4, §7).
class AppShell extends StatelessWidget {
  const AppShell({super.key, required this.shell});

  final StatefulNavigationShell shell;

  void _go(int i) => shell.goBranch(i, initialLocation: i == shell.currentIndex);

  @override
  Widget build(BuildContext context) {
    final width = widthClassOf(context);
    final c = context.colors;
    final body = Column(
      children: [
        const ConnectivityStrip(),
        // a tab's page fades in when the tab changes (Material's fade-through)
        Expanded(
          child: TabArrival(index: shell.currentIndex, child: shell),
        ),
      ],
    );
    if (width == WidthClass.compact) {
      // Material 3's navigation bar: full width on the surface, a hairline above it, the system inset beneath
      return Scaffold(
        body: body,
        bottomNavigationBar: DecoratedBox(
          decoration: BoxDecoration(
            color: c.surface,
            border: Border(top: BorderSide(color: c.border)),
          ),
          child: NavigationBar(
            selectedIndex: shell.currentIndex,
            onDestinationSelected: _go,
            destinations: [
              for (final d in _destinations)
                NavigationDestination(icon: Icon(d.icon), selectedIcon: Icon(d.selectedIcon), label: d.label),
            ],
          ),
        ),
      );
    }
    final extended = width == WidthClass.expanded;
    // the rail stands at the edge on the surface, a hairline between it and the page
    return Scaffold(
      body: SafeArea(
        child: Row(
          children: [
            DecoratedBox(
              decoration: BoxDecoration(
                color: c.surface,
                border: Border(right: BorderSide(color: c.border)),
              ),
              child: NavigationRail(
                extended: extended,
                minExtendedWidth: 232,
                labelType: extended ? NavigationRailLabelType.none : NavigationRailLabelType.all,
                selectedIndex: shell.currentIndex,
                onDestinationSelected: _go,
                leading: Padding(
                  padding: const EdgeInsets.fromLTRB(Space.sm, Space.xl, Space.sm, Space.xl),
                  child: BrandMark(height: 34, showWordmark: extended),
                ),
                destinations: [
                  for (final d in _destinations)
                    NavigationRailDestination(
                      icon: Icon(d.icon),
                      selectedIcon: Icon(d.selectedIcon),
                      label: Text(d.label),
                      padding: const EdgeInsets.symmetric(vertical: Space.xs),
                    ),
                ],
              ),
            ),
            Expanded(child: body),
          ],
        ),
      ),
    );
  }
}
