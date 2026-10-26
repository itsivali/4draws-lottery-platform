import 'package:flutter/material.dart';

/// A primary button that shows progress in place and refuses taps while busy (MASTER.md §7).
class BusyButton extends StatelessWidget {
  const BusyButton({
    super.key,
    required this.label,
    required this.onPressed,
    this.busy = false,
    this.icon,
    this.semanticsLabel,
  });

  final String label;
  final VoidCallback? onPressed;
  final bool busy;
  final IconData? icon;
  final String? semanticsLabel;

  @override
  Widget build(BuildContext context) {
    final fg = Theme.of(context).colorScheme.onPrimary;
    final child = busy
        ? SizedBox.square(dimension: 20, child: CircularProgressIndicator(strokeWidth: 2.5, color: fg))
        : Row(
            mainAxisSize: MainAxisSize.min,
            children: [
              if (icon != null) ...[Icon(icon, size: 20), const SizedBox(width: 8)],
              Flexible(child: Text(label, textAlign: TextAlign.center)),
            ],
          );
    return Semantics(
      button: true,
      enabled: onPressed != null && !busy,
      label: semanticsLabel ?? label,
      excludeSemantics: true,
      child: FilledButton(onPressed: busy ? null : onPressed, child: child),
    );
  }
}
