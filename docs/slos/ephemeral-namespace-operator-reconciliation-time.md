# Ephemeral Namespace Operator Reconciliation Time SLO

## Description

This metric tracks the reconciliation time for the Ephemeral Namespace Operator's `namespacepool`. `namespacereservation`,  
and `clowdenvironment` controllers. Reconcilations should stay
below 4 seconds for each individual controller at least 95% of the time.

## SLI Rationale

High reconciliation times backup the queue of objects needed to be reconciled. This could indicate an issue with the operator
and prevent objects from getting added or updated in a timely manor.

## Implmentation

The Operator SDK exposes the `controller_runtime_reconcile_time_seconds_bucket` metric to show reconciliation times. Using the
`sum(average_over_time)` modifier allows us to determine if that amount is staying under 4 seconds.

## SLO Rationale
Almost all reconciler calls should be handled without issue in a timely manor. If we are hitting reconciliation times greater than
4 seconds, debugging should begin.

## Alerting
Alerts should be kept to a medium level. Because there are a myriad of issues that could cause high reconciliation times, breaking
this SLO should not result in a page. It should be addressed, but higher than normal reconciliation times alone does not indiciate
an outage.
