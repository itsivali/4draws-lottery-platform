import 'package:flutter/material.dart';
import 'package:flutter/services.dart';

/// The date of birth every new account gives, with its 18+ declaration (ADR 0029). Typed as on the ID card
/// (DD/MM/YYYY); the server checks it again and refuses anyone under 18.
class DateOfBirthField extends StatelessWidget {
  const DateOfBirthField({super.key, required this.controller});

  final TextEditingController controller;

  /// The date as the API takes it (YYYY-MM-DD), or null when [v] is not a real DD/MM/YYYY date.
  static String? iso(String v) {
    final m = RegExp(r'^(\d{1,2})/(\d{1,2})/(\d{4})$').firstMatch(v.trim());
    if (m == null) return null;
    final d = int.parse(m.group(1)!), mo = int.parse(m.group(2)!), y = int.parse(m.group(3)!);
    final date = DateTime.utc(y, mo, d);
    if (date.day != d || date.month != mo || date.year != y || y < 1900) return null;
    String two(int n) => n.toString().padLeft(2, '0');
    return '$y-${two(mo)}-${two(d)}';
  }

  /// Null when [v] is the date of birth of someone 18 or older today, else what to tell the player.
  static String? error(String? v, {DateTime? today}) {
    final s = iso(v ?? '');
    if (s == null) return 'Enter your date of birth as DD/MM/YYYY.';
    final dob = DateTime.parse(s);
    final now = today ?? DateTime.now();
    if (dob.isAfter(now)) return 'Enter your date of birth as DD/MM/YYYY.';
    var age = now.year - dob.year;
    if (now.month < dob.month || (now.month == dob.month && now.day < dob.day)) age--;
    return age < 18 ? 'You must be 18 or older to play.' : null;
  }

  @override
  Widget build(BuildContext context) => TextFormField(
    controller: controller,
    keyboardType: TextInputType.datetime,
    autofillHints: const [AutofillHints.birthday],
    textInputAction: TextInputAction.next,
    inputFormatters: [FilteringTextInputFormatter.allow(RegExp(r'[0-9/]')), LengthLimitingTextInputFormatter(10)],
    decoration: const InputDecoration(
      labelText: 'Date of birth',
      helperText: 'DD/MM/YYYY, as on your ID. You must be 18 or older.',
      prefixIcon: Icon(Icons.cake_rounded),
    ),
    validator: error,
  );
}
