import 'package:flutter/material.dart';

import '../formatting/money.dart';

/// An amount in shillings, formatted one way everywhere.
class MoneyText extends StatelessWidget {
  const MoneyText(this.amountKes, {super.key, this.style, this.signed = false, this.color});

  final int amountKes;
  final TextStyle? style;
  final bool signed;
  final Color? color;

  @override
  Widget build(BuildContext context) {
    final base = style ?? Theme.of(context).textTheme.titleLarge;
    return Text(
      formatKes(amountKes, signed: signed),
      style: base?.copyWith(color: color),
    );
  }
}
