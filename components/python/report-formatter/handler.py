"""report-formatter is a Numaflow mapper UDF written in Python to demonstrate
mixing languages in one pipeline: it only reformats the JSON FlashResult
produced upstream by the Go udsflasher vertex into a human-readable report
line, with no protocol logic of its own.
"""

import json

from pynumaflow.mapper import Datum, Mapper, MapServer, Message, Messages


class ReportFormatter(Mapper):
    def handler(self, keys: list[str], datum: Datum) -> Messages:
        try:
            result = json.loads(datum.value)
        except json.JSONDecodeError:
            return Messages(Message.to_drop())

        job_id = result.get("job_id", "?")
        ecu_id = result.get("ecu_id", "?")
        status = result.get("status", "unknown")
        duration_ms = result.get("duration_ms", 0)

        if status == "ok":
            report = f"[OK] job={job_id} ecu={ecu_id} flashed in {duration_ms}ms"
        else:
            error = result.get("error", "unknown error")
            report = f"[FAIL] job={job_id} ecu={ecu_id} after {duration_ms}ms: {error}"

        return Messages(Message(value=report.encode("utf-8"), keys=keys))


if __name__ == "__main__":
    MapServer(ReportFormatter()).start()
