import 'package:flutter/material.dart';
import 'package:flutter/services.dart';

import '../../../app/theme/tokens.dart';

/// First name, second name, then the national ID number (owner, 2026-10-30): asked of every new account, kept, and
/// checked against the ID before the first withdrawal. An account from before they were asked gives them from its
/// account (owner, 2026-10-31) with the same fields.
class IdentityFields extends StatelessWidget {
  const IdentityFields({
    super.key,
    required this.firstName,
    required this.secondName,
    required this.nationalId,
    this.error,
  });

  final TextEditingController firstName;
  final TextEditingController secondName;
  final TextEditingController nationalId;
  final String? error;

  static final _name = RegExp(r"^\p{L}[\p{L} .'-]{1,99}$", unicode: true);

  /// Null when [v] is a name as on an ID card (a letter, then letters, spaces, dots, apostrophes or hyphens), else
  /// what to tell the player; the server applies the same rule.
  static String? nameError(String? v, String which) {
    final s = (v ?? '').trim().replaceAll(RegExp(r'\s+'), ' ');
    if (s.isEmpty) return 'Enter your $which name as on your ID.';
    return _name.hasMatch(s) ? null : 'Use the letters of your $which name as on your ID.';
  }

  @override
  Widget build(BuildContext context) => Column(
    crossAxisAlignment: CrossAxisAlignment.stretch,
    children: [
      TextFormField(
        controller: firstName,
        autofillHints: const [AutofillHints.givenName],
        textCapitalization: TextCapitalization.words,
        textInputAction: TextInputAction.next,
        decoration: const InputDecoration(labelText: 'First name', prefixIcon: Icon(Icons.badge_rounded)),
        validator: (v) => nameError(v, 'first'),
      ),
      const SizedBox(height: Space.lg),
      TextFormField(
        controller: secondName,
        autofillHints: const [AutofillHints.familyName],
        textCapitalization: TextCapitalization.words,
        textInputAction: TextInputAction.next,
        decoration: const InputDecoration(
          labelText: 'Second name',
          helperText: 'Both names as on your ID, as M-Pesa shows them',
          prefixIcon: Icon(Icons.badge_outlined),
        ),
        validator: (v) => nameError(v, 'second'),
      ),
      const SizedBox(height: Space.lg),
      TextFormField(
        controller: nationalId,
        keyboardType: TextInputType.number,
        textInputAction: TextInputAction.next,
        inputFormatters: [FilteringTextInputFormatter.digitsOnly, LengthLimitingTextInputFormatter(9)],
        decoration: InputDecoration(
          labelText: 'National ID number',
          helperText: 'Checked before your first withdrawal',
          errorText: error,
          errorMaxLines: 3,
          prefixIcon: const Icon(Icons.credit_card_rounded),
        ),
        validator: (v) =>
            RegExp(r'^[0-9]{7,9}$').hasMatch((v ?? '').trim()) ? null : 'Enter the 7 to 9 digits of your ID number.',
      ),
    ],
  );
}
