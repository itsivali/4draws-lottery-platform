/// "254712345678" → "0712 345 678".
String formatMsisdn(String msisdn) {
  final m = RegExp(r'^254([17]\d{2})(\d{3})(\d{3})$').firstMatch(msisdn);
  return m == null ? msisdn : '0${m[1]} ${m[2]} ${m[3]}';
}

/// A light check before sending; the API normalises and validates the number itself.
bool isLikelyKenyanMobile(String input) {
  final d = input.replaceAll(RegExp(r'[\s()-]'), '');
  return RegExp(r'^(\+?254|0)[17]\d{8}$').hasMatch(d);
}
