import 'package:flutter/material.dart';
import 'package:flutter/services.dart';

import '../../../app/theme/tokens.dart';
import '../../../core/formatting/money.dart';

/// A whole-shilling amount with quick picks, validated against the API's own range.
class AmountField extends StatelessWidget {
  const AmountField({
    super.key,
    required this.controller,
    required this.min,
    required this.max,
    required this.quick,
    required this.label,
    this.helper,
    this.extraValidator,
    this.enabled = true,
  });

  final TextEditingController controller;
  final int min;
  final int max;
  final List<int> quick;
  final String label;
  final String? helper;
  final String? Function(int)? extraValidator;
  final bool enabled;

  @override
  Widget build(BuildContext context) => Column(
    crossAxisAlignment: CrossAxisAlignment.stretch,
    children: [
      TextFormField(
        controller: controller,
        enabled: enabled,
        keyboardType: TextInputType.number,
        inputFormatters: [FilteringTextInputFormatter.digitsOnly, LengthLimitingTextInputFormatter(6)],
        style: Theme.of(context).textTheme.titleLarge,
        decoration: InputDecoration(
          labelText: label,
          prefixText: 'KES ',
          helperText: helper ?? '${formatKes(min)} to ${formatKes(max)}, whole shillings',
        ),
        validator: (v) {
          final n = int.tryParse(v ?? '');
          if (n == null) return 'Enter an amount.';
          if (n < min || n > max) return 'Enter ${formatKes(min)} to ${formatKes(max)}.';
          return extraValidator?.call(n);
        },
      ),
      const SizedBox(height: Space.md),
      Wrap(
        spacing: Space.sm,
        runSpacing: Space.sm,
        children: [
          for (final q in quick)
            ActionChip(label: Text(formatKes(q)), onPressed: enabled ? () => controller.text = '$q' : null),
        ],
      ),
    ],
  );
}
