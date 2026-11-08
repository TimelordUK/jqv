# A jq program laid out as a script: helpers first, then one pipeline at the bottom that
# is the program. A summary of the teams in people.json: how many people, how many are
# over an age given on the command line, the average age, and who is in each team.
#
#   jq --arg min 30 -f query.jq input.json
#
# Things to notice:
#   - def names a piece of pipeline. A `$x` parameter is a value; a bare `f` parameter is
#     a filter, run on each item, the way map(f) and sort_by(f) work.
#   - `X as $v | ...` names a value for the rest of the pipe. It can't be changed, and it
#     leaves `.` as it was, so each set below is computed from the same input.
#   - --arg values arrive as strings, so $min goes through tonumber (--argjson would not).

# ---- helpers ----------------------------------------------------------------------

# Every person, one at a time, with their team copied in: the documents' people are nested
# under their team, so the team is named first and added to each.
def people: .[] | .team as $t | .people[] | . + {team: $t};

# Only the people older than $age.
def older($age): select(.age > $age);

# The average of f over an array, or null for an empty one (rather than dividing by zero).
def avg(f): if length == 0 then null else (map(f) | add) / length end;

# ---- the program ------------------------------------------------------------------

($min | tonumber) as $min             # one name, now a number
| [people] as $all                    # data set 1: everyone, flattened
| ($all | map(older($min))) as $over  # data set 2: computed from set 1
| {                                   # glue them together into one result
    total:   ($all | length),
    over:    ($over | length),
    min_age: $min,
    avg_age: ($all | avg(.age)),
    by_team: ($all | group_by(.team) | map({key: .[0].team, value: map(.name)}) | from_entries)
  }
