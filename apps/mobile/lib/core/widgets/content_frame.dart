import 'package:flutter/material.dart';

import '../../app/theme/tokens.dart';

enum WidthClass { compact, medium, expanded }

WidthClass widthClassOf(BuildContext context) {
  final w = MediaQuery.sizeOf(context).width;
  if (w >= Breakpoints.expanded) return WidthClass.expanded;
  if (w >= Breakpoints.medium) return WidthClass.medium;
  return WidthClass.compact;
}

/// Caps content width so a phone layout is never stretched across a desktop browser (MASTER.md §4).
class ContentFrame extends StatelessWidget {
  const ContentFrame({super.key, required this.child, this.wide = false});

  final Widget child;

  /// Two-column screens (Home, Play) may use the expanded width.
  final bool wide;

  @override
  Widget build(BuildContext context) {
    final max = switch (widthClassOf(context)) {
      WidthClass.compact => double.infinity,
      WidthClass.medium => Breakpoints.contentMedium,
      WidthClass.expanded => wide ? Breakpoints.contentExpanded : Breakpoints.contentMedium,
    };
    return Align(
      alignment: Alignment.topCenter,
      child: ConstrainedBox(
        constraints: BoxConstraints(maxWidth: max),
        child: child,
      ),
    );
  }
}

/// The standard scrolling page: a title, gutters, capped width.
class PageScaffold extends StatelessWidget {
  const PageScaffold({
    super.key,
    required this.title,
    required this.children,
    this.actions,
    this.wide = false,
    this.onRefresh,
  });

  final String title;
  final List<Widget> children;
  final List<Widget>? actions;
  final bool wide;
  final Future<void> Function()? onRefresh;

  @override
  Widget build(BuildContext context) {
    final list = ListView(
      padding: const EdgeInsets.fromLTRB(Space.lg, Space.sm, Space.lg, Space.xxl),
      children: [
        ContentFrame(
          wide: wide,
          child: Column(crossAxisAlignment: CrossAxisAlignment.stretch, children: _spaced(children)),
        ),
      ],
    );
    return Scaffold(
      appBar: AppBar(
        title: Semantics(header: true, child: Text(title)),
        actions: actions,
      ),
      body: SafeArea(
        top: false,
        child: onRefresh == null ? list : RefreshIndicator(onRefresh: onRefresh!, child: list),
      ),
    );
  }

  static List<Widget> _spaced(List<Widget> items) => [
    for (var i = 0; i < items.length; i++) ...[if (i > 0) const SizedBox(height: Space.lg), items[i]],
  ];
}
