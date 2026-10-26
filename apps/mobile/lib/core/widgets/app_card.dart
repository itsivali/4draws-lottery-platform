import 'package:flutter/material.dart';

import '../../app/theme/tokens.dart';

/// The one card: surface, hairline border, no shadow (MASTER.md §5, §7).
class AppCard extends StatelessWidget {
  const AppCard({
    super.key,
    this.title,
    this.trailing,
    required this.child,
    this.padding = const EdgeInsets.all(Space.lg),
  });

  final String? title;
  final Widget? trailing;
  final Widget child;
  final EdgeInsets padding;

  @override
  Widget build(BuildContext context) {
    final t = Theme.of(context).textTheme;
    return Card(
      child: Padding(
        padding: padding,
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.stretch,
          mainAxisSize: MainAxisSize.min,
          children: [
            if (title != null) ...[
              Row(
                children: [
                  Expanded(
                    child: Semantics(header: true, child: Text(title!, style: t.titleMedium)),
                  ),
                  ?trailing,
                ],
              ),
              const SizedBox(height: Space.md),
            ],
            child,
          ],
        ),
      ),
    );
  }
}
