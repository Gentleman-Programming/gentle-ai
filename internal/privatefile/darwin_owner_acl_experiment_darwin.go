//go:build darwin && cgo && privatefileowneraclexperiment

package privatefile

/*
#include <sys/acl.h>
#include <membership.h>
#include <uuid/uuid.h>
#include <errno.h>
#include <stdio.h>
#include <stdlib.h>
#include <unistd.h>

// These helpers deliberately return errors instead of treating NULL/ENOENT as
// an empty ACL. No child ACL is ever read by pathname.
static char acl_error[160];
static const char *fail_acl(const char *stage) {
    snprintf(acl_error, sizeof(acl_error), "%s: errno=%d", stage, errno);
    return acl_error;
}

static const char *new_ace(acl_t *acl, const uuid_t who, int inherit) {
    acl_entry_t entry;
    acl_permset_t perms;
    acl_flagset_t flags;
    if (acl_create_entry(acl, &entry) != 0)
        return fail_acl("acl_create_entry");
    if (acl_set_tag_type(entry, ACL_EXTENDED_ALLOW) != 0)
        return fail_acl("acl_set_tag_type");
    if (acl_set_qualifier(entry, who) != 0)
        return fail_acl("acl_set_qualifier");
    if (acl_get_permset(entry, &perms) != 0)
        return fail_acl("acl_get_permset(new)");
    if (acl_clear_perms(perms) != 0 ||
        acl_add_perm(perms, ACL_READ_DATA) != 0 ||
        acl_add_perm(perms, ACL_WRITE_DATA) != 0 ||
        acl_set_permset(entry, perms) != 0) return fail_acl("set read/write perms");
    if (acl_get_flagset_np(entry, &flags) != 0)
        return fail_acl("acl_get_flagset_np(new)");
    if (acl_clear_flags_np(flags) != 0)
        return fail_acl("acl_clear_flags_np");
    if (inherit && (acl_add_flag_np(flags, ACL_ENTRY_FILE_INHERIT) != 0 ||
                    acl_add_flag_np(flags, ACL_ENTRY_DIRECTORY_INHERIT) != 0))
        return fail_acl("add inherit flags");
    if (acl_set_flagset_np(entry, flags) != 0)
        return fail_acl("acl_set_flagset_np");
    return NULL;
}

static const char *parent_everyone_acl(const char *path) {
    // Apple's well-known everyone UUID (not a UID/GID mapping).
    uuid_t everyone;
    if (uuid_parse("ABCDEFAB-CDEF-ABCD-EFAB-CDEF0000000C", everyone) != 0)
        return fail_acl("everyone UUID parse");
    acl_t acl = acl_init(1);
    if (acl == NULL) return fail_acl("acl_init(parent)");
    const char *error = new_ace(&acl, everyone, 1);
    if (error == NULL && acl_valid(acl) != 0)
        error = fail_acl("acl_valid(parent)");
    if (error == NULL && acl_set_file(path, ACL_TYPE_EXTENDED, acl) != 0)
        error = fail_acl("acl_set_file(parent)");
    if (acl_free(acl) != 0 && error == NULL)
        error = fail_acl("acl_free(parent)");
    return error;
}

static const char *owner_uuid(uid_t owner, uuid_t owner_uuid) {
    if (owner != getuid())
        return fail_acl("fstat owner != getuid");
    // Membership functions return an error code rather than setting errno.
    int result = mbr_uid_to_uuid(owner, owner_uuid);
    if (result != 0) {
        errno = result;
        return fail_acl("mbr_uid_to_uuid");
    }
    id_t mapped;
    int type;
    result = mbr_uuid_to_id(owner_uuid, &mapped, &type);
    if (result != 0) {
        errno = result;
        return fail_acl("mbr_uuid_to_id");
    }
    if (type != ID_TYPE_UID || mapped != owner)
        return fail_acl("owner UUID round trip mismatch");
    return NULL;
}

static const char *check_ace(acl_entry_t entry, const uuid_t expected, int inherited) {
    acl_tag_t tag;
    if (acl_get_tag_type(entry, &tag) != 0 || tag != ACL_EXTENDED_ALLOW)
        return fail_acl("allow tag");
    void *qualifier = acl_get_qualifier(entry);
    if (qualifier == NULL)
        return fail_acl("acl_get_qualifier");
    int equal = uuid_compare((unsigned char *)qualifier, expected) == 0;
    if (acl_free(qualifier) != 0)
        return fail_acl("acl_free(qualifier)");
    if (!equal)
        return fail_acl("ACE UUID mismatch");
    acl_permset_t perms;
    if (acl_get_permset(entry, &perms) != 0)
        return fail_acl("acl_get_permset(read)");
    // Check the whole documented permission vocabulary, not just the two
    // desired bits: an extra grant would invalidate the one-owner proof.
    const acl_perm_t all[] = {
        ACL_READ_DATA, ACL_WRITE_DATA, ACL_EXECUTE, ACL_DELETE,
        ACL_APPEND_DATA, ACL_DELETE_CHILD, ACL_READ_ATTRIBUTES,
        ACL_WRITE_ATTRIBUTES, ACL_READ_EXTATTRIBUTES, ACL_WRITE_EXTATTRIBUTES,
        ACL_READ_SECURITY, ACL_WRITE_SECURITY, ACL_CHANGE_OWNER, ACL_SYNCHRONIZE
    };
    for (unsigned i = 0; i < sizeof(all)/sizeof(all[0]); i++) {
        int got = acl_get_perm_np(perms, all[i]);
        if (got < 0 || got != (i < 2))
            return fail_acl("ACE permission mismatch");
    }
    acl_flagset_t flags;
    if (acl_get_flagset_np(entry, &flags) != 0)
        return fail_acl("acl_get_flagset_np(read)");
    const acl_flag_t all_flags[] = {
        ACL_ENTRY_INHERITED, ACL_ENTRY_FILE_INHERIT,
        ACL_ENTRY_DIRECTORY_INHERIT, ACL_ENTRY_LIMIT_INHERIT,
        ACL_ENTRY_ONLY_INHERIT
    };
    for (unsigned i = 0; i < sizeof(all_flags)/sizeof(all_flags[0]); i++) {
        int got = acl_get_flag_np(flags, all_flags[i]);
        if (got < 0 || got != (i == 0 && inherited))
            return fail_acl("ACE flag mismatch");
    }
    return NULL;
}

static const char *read_exact_acl(int fd, const uuid_t expected, int inherited) {
    acl_t acl = acl_get_fd_np(fd, ACL_TYPE_EXTENDED);
    if (acl == NULL)
        return fail_acl("acl_get_fd_np: NULL is inconclusive");
    const char *error = NULL;
    acl_entry_t entry;
    if (acl_valid(acl) != 0)
        error = fail_acl("acl_valid(read)");
    if (error == NULL && acl_get_entry(acl, ACL_FIRST_ENTRY, &entry) != 0)
        error = fail_acl("acl_get_entry(first)");
    if (error == NULL)
        error = check_ace(entry, expected, inherited);
    if (error == NULL) {
        // acl_get_entry(3): 0 is an entry, -1/EINVAL after the last
        // ACL_NEXT_ENTRY is the documented end for a validated ACL.
        errno = 0;
        int next = acl_get_entry(acl, ACL_NEXT_ENTRY, &entry);
        if (next != -1 || errno != EINVAL)
            error = fail_acl("acl_get_entry: extra ACE or ambiguous end");
    }
    if (acl_free(acl) != 0 && error == NULL)
        error = fail_acl("acl_free(read)");
    return error;
}

static const char *replace_owner_acl(int fd, uid_t owner) {
    uuid_t who;
    const char *error = owner_uuid(owner, who);
    if (error != NULL)
        return error;
    acl_t acl = acl_init(1);
    if (acl == NULL)
        return fail_acl("acl_init(owner)");
    error = new_ace(&acl, who, 0);
    if (error == NULL && acl_valid(acl) != 0)
        error = fail_acl("acl_valid(owner)");
    if (error == NULL && acl_set_fd_np(fd, acl, ACL_TYPE_EXTENDED) != 0)
        error = fail_acl("acl_set_fd_np(owner)");
    if (acl_free(acl) != 0 && error == NULL)
        error = fail_acl("acl_free(owner)");
    if (error != NULL)
        return error;
    return read_exact_acl(fd, who, 0);
}

static const char *read_inherited_acl(int fd) {
    uuid_t everyone;
    if (uuid_parse("ABCDEFAB-CDEF-ABCD-EFAB-CDEF0000000C", everyone) != 0)
        return fail_acl("everyone UUID parse(read)");
    return read_exact_acl(fd, everyone, 1);
}
*/
import "C"

import "unsafe"

func experimentParentACL(path string) string {
	cpath := C.CString(path)
	defer C.free(unsafe.Pointer(cpath))
	return experimentError(C.parent_everyone_acl(cpath))
}

func experimentInheritedACL(fd uintptr) string {
	return experimentError(C.read_inherited_acl(C.int(fd)))
}

func experimentReplaceOwnerACL(fd uintptr, owner uint32) string {
	return experimentError(C.replace_owner_acl(C.int(fd), C.uid_t(owner)))
}

func experimentError(message *C.char) string {
	if message == nil {
		return ""
	}
	return C.GoString(message)
}
