"""Compatibility readers for source-neutral import candidate fields."""


def as_text(value):
    if value is None:
        return ""
    return str(value)


def dependency_source_ids(candidate):
    predecessor = as_text(candidate.get("predecessor_source_id"))
    successor = as_text(candidate.get("successor_source_id"))
    if predecessor and successor:
        return predecessor, successor
    from_id = as_text(candidate.get("from_work_package_id"))
    to_id = as_text(candidate.get("to_work_package_id"))
    if as_text(candidate.get("source")) in ("", "openproject") and as_text(
        candidate.get("relation_type")
    ) in ("follows", "blocked"):
        from_id, to_id = to_id, from_id
    return predecessor or from_id, successor or to_id


def dependency_key(dependency_type, predecessor, successor):
    """Return the comparison key; relates is symmetric."""
    dependency_type = as_text(dependency_type)
    predecessor = as_text(predecessor)
    successor = as_text(successor)
    if dependency_type == "relates" and successor < predecessor:
        predecessor, successor = successor, predecessor
    return dependency_type, predecessor, successor


def source_object_id(candidate):
    return as_text(candidate.get("source_object_id")) or as_text(
        candidate.get("work_package_id")
    )
