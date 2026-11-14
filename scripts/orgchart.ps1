# Writes a made-up org chart as JSON: a flat array of employees that link to each other by
# id, for trying graph queries in jqv (a tree from the flat list, everyone below someone,
# the chain up to the top).
#
#   ./scripts/orgchart.ps1                          # testdata/orgchart.json, 200 people
#   ./scripts/orgchart.ps1 -Count 2000 -Out big.json
#
# Each employee:
#   {"id": "E0007", "name": ..., "title": ..., "department": ..., "location": ...,
#    "reports_to": ["E0002"], "direct_reports": ["E0031", "E0032"]}
#
# - reports_to is always an array. The first id is the line manager; a few people (about
#   -Matrix of them) also have a dotted line to a manager in another department.
# - direct_reports is the mirror of reports_to: X is in Y's direct_reports exactly when Y
#   is in X's reports_to, dotted lines included. The CEO's reports_to is [].
# - Managers always sit one level above their reports, so the graph has no cycles, but
#   with dotted lines it is not a tree: someone can be reached from the top by two paths.
# - The same -Seed gives the same file, so tests and notes can name ids.
param(
    [int]$Count = 200,
    [int]$Seed = 42,
    [double]$Matrix = 0.06,
    [string]$Out = (Join-Path (Split-Path $PSScriptRoot -Parent) 'testdata/orgchart.json')
)
$ErrorActionPreference = 'Stop'
$rng = [Random]::new($Seed)
function Pick($items) { $items[$rng.Next($items.Count)] }

$first = @('Ada', 'Ben', 'Cara', 'Dev', 'Elena', 'Femi', 'Grace', 'Hiro', 'Ines', 'Jon',
    'Kofi', 'Lena', 'Mateo', 'Nina', 'Omar', 'Priya', 'Quinn', 'Rosa', 'Sam', 'Tariq',
    'Uma', 'Victor', 'Wen', 'Xavi', 'Yara', 'Zane', 'Aisha', 'Bruno', 'Chloe', 'Dmitri')
$last = @('Adams', 'Baker', 'Chen', 'Diaz', 'Evans', 'Fischer', 'Garcia', 'Hughes', 'Ito',
    'Jones', 'Khan', 'Lopez', 'Moreau', 'Nakamura', 'Okafor', 'Patel', 'Quint', 'Rossi',
    'Singh', 'Taylor', 'Ueda', 'Varga', 'Walsh', 'Xu', 'Young', 'Zhang')
$locations = @('London', 'New York', 'Singapore', 'Berlin', 'Toronto')
$staff = [ordered]@{
    Engineering = @('Software Engineer', 'Senior Software Engineer', 'SRE', 'QA Engineer')
    Sales       = @('Account Executive', 'Sales Engineer', 'Account Manager')
    Finance     = @('Analyst', 'Accountant', 'Controller')
    Operations  = @('Operations Analyst', 'Support Engineer', 'Trade Support')
    Marketing   = @('Marketing Manager', 'Content Writer', 'Designer')
    People      = @('Recruiter', 'HR Partner', 'Payroll Specialist')
}
# How many reports someone at each level gets: [min, max]. Level 4 is the last.
$fanout = @(@($staff.Count, $staff.Count), @(2, 3), @(2, 3), @(3, 5))

$all = [System.Collections.Generic.List[object]]::new()
$level = @{}
function New-Employee($title, $dept, $lvl, $manager) {
    $e = [ordered]@{
        id             = 'E{0:D4}' -f ($all.Count + 1)
        name           = "$(Pick $first) $(Pick $last)"
        title          = $title
        department     = $dept
        location       = Pick $locations
        reports_to     = [System.Collections.Generic.List[string]]::new()
        direct_reports = [System.Collections.Generic.List[string]]::new()
    }
    if ($manager) {
        $e.reports_to.Add($manager.id)
        $manager.direct_reports.Add($e.id)
    }
    $all.Add($e)
    $level[$e.id] = $lvl
    $e
}

# Breadth first from the CEO, so every level fills before the next and -Count cuts the
# bottom level short rather than leaving a branch empty.
$queue = [System.Collections.Generic.Queue[object]]::new()
$queue.Enqueue((New-Employee 'Chief Executive Officer' 'Executive' 0 $null))
$depts = @($staff.Keys)
while ($queue.Count -and $all.Count -lt $Count) {
    $m = $queue.Dequeue()
    $lvl = $level[$m.id] + 1
    if ($lvl -gt $fanout.Count) { continue }
    $n = $rng.Next($fanout[$lvl - 1][0], $fanout[$lvl - 1][1] + 1)
    for ($i = 0; $i -lt $n -and $all.Count -lt $Count; $i++) {
        $dept = if ($lvl -eq 1) { $depts[$i] } else { $m.department }
        $title = switch ($lvl) {
            1 { "VP $dept" }
            2 { "Director, $dept" }
            3 { "$dept Manager" }
            default { Pick $staff[$dept] }
        }
        $queue.Enqueue((New-Employee $title $dept $lvl $m))
    }
}

# Dotted lines: a second manager one level up, in another department. Always one level
# up, so no cycles.
foreach ($e in $all) {
    $lvl = $level[$e.id]
    if ($lvl -lt 2 -or $rng.NextDouble() -ge $Matrix) { continue }
    $others = @($all | Where-Object { $level[$_.id] -eq $lvl - 1 -and $_.department -ne $e.department })
    if (-not $others) { continue }
    $m = Pick $others
    $e.reports_to.Add($m.id)
    $m.direct_reports.Add($e.id)
}

ConvertTo-Json -InputObject $all -Depth 4 | Set-Content -Path $Out -Encoding utf8NoBOM
$dotted = @($all | Where-Object { $_.reports_to.Count -gt 1 }).Count
"wrote $Out`: $($all.Count) employees, $dotted with a dotted line"
