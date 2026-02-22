package Manju.manju_backend_middleware.Repositories;

import Manju.manju_backend_middleware.Models.User;
import org.springframework.data.jpa.repository.JpaRepository;

import java.util.Optional;

public interface UserRepository extends JpaRepository<User,Long> {

    Optional<User> findByUsername(String username);
}
