import 'package:flutter/material.dart';
import 'package:go_router/go_router.dart';

import '../core/widgets/brand_mark.dart';
import '../core/widgets/content_frame.dart';
import 'connectivity_strip.dart';
import 'theme/tokens.dart';

class _Destination {
  const _Destination(this.label, this.icon, this.selectedIcon);

  final String label;
  final IconData icon;
  final IconData selectedIcon;
}

const _destinations = [
  _Destination('Home', Icons.home_outlined, Icons.home_rounded),
  _Destination('Play', Icons.donut_large_outlined, Icons.donut_large_rounded),
  _Destination('Draws', Icons.event_outlined, Icons.event_rounded),
  _Destination('Wallet', Icons.account_balance_wallet_outlined, Icons.account_balance_wallet_rounded),
  _Destination('Account', Icons.person_outline_rounded, Icons.person_rounded),
];

/// Five top-level destinations: a bottom bar on phones, a rail on tablets, an extended rail with the wordmark on
/// laptops and desktops (MASTER.md §4, §7).
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
        Expanded(child: shell),
      ],
    );
    if (width == WidthClass.compact) {
      return Scaffold(
        body: body,
        bottomNavigationBar: DecoratedBox(
          decoration: BoxDecoration(
            color: c.surface,
            border: Border(top: BorderSide(color: c.border)),
            boxShadow: Elevation.bar(c),
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
    return Scaffold(
      body: SafeArea(
        child: Row(
          children: [
            NavigationRail(
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
                    padding: const EdgeInsets.symmetric(vertical: Space.xxs),
                  ),
              ],
            ),
            VerticalDivider(width: 1, color: c.border),
            Expanded(child: body),
          ],
        ),
      ),
    );
  }
}
