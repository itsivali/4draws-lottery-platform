import 'api_client.dart';
import 'api_failure.dart';
import 'json.dart';

/// Reads a typed model from an answer; a body that does not match the contract becomes a [MalformedResponseFailure].
T parse<T>(ApiResponse r, T Function(JsonObject) read) {
  try {
    return read(JsonObject(r.json));
  } on FormatException catch (e) {
    throw MalformedResponseFailure(r.status, e.message);
  } on ArgumentError catch (e) {
    throw MalformedResponseFailure(r.status, '${e.message}'); // an enum value the app does not know
  }
}
